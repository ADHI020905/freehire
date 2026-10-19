package ojcpmcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/api/ojcp"
)

// semanticReader answers search_jobs with `applied` whenever the agent asked, and records
// whether the flag survived the transport.
type semanticReader struct {
	recordingReader
	asked bool
}

func (r *semanticReader) SearchJobs(_ context.Context, in ojcp.SearchInput) (ojcp.SearchJobsResponse, error) {
	r.asked = in.WantsSemantic()
	resp := ojcp.SearchJobsResponse{Query: in.Query}
	if r.asked {
		resp.Semantic = ojcp.SemanticApplied
	}
	return resp.Finalize(), nil
}

func connectOJCP(t *testing.T, r Reader) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server, err := newServer(r).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// An agent discovers the extension from the tool list, or not at all.
func TestSearchJobsAdvertisesTheSemanticField(t *testing.T) {
	session := connectOJCP(t, &semanticReader{})

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range tools.Tools {
		if tool.Name != toolSearchJobs {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal input schema: %v", err)
		}
		// Properties stay raw: candidate_context is `any`, whose schema is the bare `true`.
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("read input schema: %v", err)
		}
		rawProp, ok := schema.Properties["semantic"]
		if !ok {
			t.Fatalf("search_jobs input schema has no semantic property: %s", raw)
		}
		var prop struct {
			Type        any    `json:"type"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(rawProp, &prop); err != nil {
			t.Fatalf("read semantic property: %v", err)
		}
		if b, _ := json.Marshal(prop.Type); string(b) != `"boolean"` && string(b) != `["null","boolean"]` {
			t.Errorf("semantic type = %s, want boolean", b)
		}
		if prop.Description == "" {
			t.Error("semantic carries no description; an agent reading the schema cannot tell what it does")
		}
		for _, name := range schema.Required {
			if name == "semantic" {
				t.Error("semantic is required; it must stay optional")
			}
		}
		return
	}
	t.Fatalf("no %s tool listed", toolSearchJobs)
}

// The flag reaches the reader, and a response carrying the extension passes the SDK's own
// output validation — which rejects every call when the schema and the value disagree.
func TestSearchJobsCarriesTheSemanticFlagBothWays(t *testing.T) {
	reader := &semanticReader{}
	session := connectOJCP(t, reader)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolSearchJobs,
		Arguments: map[string]any{"query": "payments", "semantic": true},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned an error result: %+v", res.Content)
	}
	if !reader.asked {
		t.Error("the semantic flag did not reach the reader")
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out ojcp.SearchJobsResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("read structured content: %v", err)
	}
	if out.Semantic != ojcp.SemanticApplied {
		t.Errorf("semantic = %q, want %q", out.Semantic, ojcp.SemanticApplied)
	}
}

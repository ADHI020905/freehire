package mcpapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// callJSON calls one tool and decodes the JSON ChatGPT's deep-research connector reads —
// the TEXT content, not the structured content: that connector's contract is "one text item
// holding a JSON object", and a server that answers only in structuredContent answers it
// nothing.
func callJSON(t *testing.T, session *mcp.ClientSession, name string, args map[string]any, into any) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s answered an error: %+v", name, res.Content)
	}
	if len(res.Content) != 1 {
		t.Fatalf("%s returned %d content items, want exactly one", name, len(res.Content))
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s content is %T, want text", name, res.Content[0])
	}
	if err := json.Unmarshal([]byte(text.Text), into); err != nil {
		t.Fatalf("%s text is not JSON: %v (%q)", name, err, text.Text)
	}
}

func TestDeepResearchSearchAnswersInTheConnectorsFixedShape(t *testing.T) {
	// search(query) -> {results: [{id, title, url}]} is fixed by ChatGPT's deep-research and
	// company-knowledge connectors; a server answering in any other shape is skipped by them.
	session := connect(t, readerThatFinds(t))

	var got struct {
		Results []struct{ ID, Title, URL string } `json:"results"`
	}
	callJSON(t, session, toolSearch, map[string]any{"query": "go"}, &got)

	if len(got.Results) != 1 {
		t.Fatalf("results = %+v, want the one posting", got.Results)
	}
	hit := got.Results[0]
	if hit.ID != "senior-go-engineer-acme-abc123" {
		t.Errorf("id = %q, want the slug fetch takes", hit.ID)
	}
	if !strings.Contains(hit.Title, "Senior Go Engineer") || !strings.Contains(hit.Title, "Acme") {
		t.Errorf("title = %q, want the role and the employer", hit.Title)
	}
	if hit.URL != testOrigin+"/jobs/senior-go-engineer-acme-abc123" {
		t.Errorf("url = %q, want the posting's freehire page", hit.URL)
	}
}

func TestDeepResearchFetchReturnsThePostingInFull(t *testing.T) {
	session := connect(t, readerThatFinds(t))

	var got struct {
		ID, Title, Text, URL string
		Metadata             map[string]any `json:"metadata"`
	}
	callJSON(t, session, toolFetch, map[string]any{"id": "senior-go-engineer-acme-abc123"}, &got)

	if got.ID != "senior-go-engineer-acme-abc123" || got.URL == "" {
		t.Errorf("id/url = %q/%q, want the slug and the posting page", got.ID, got.URL)
	}
	if !strings.Contains(got.Text, "We need") || strings.Contains(got.Text, "<p>") {
		t.Errorf("text = %q, want the full description, markup stripped", got.Text)
	}
	// Attribution travels with the record here exactly as it does on get_job: a research
	// report that quotes a posting without its board reads as though freehire were the
	// employer.
	if got.Metadata["source"] != "greenhouse" || got.Metadata["company"] != "Acme" {
		t.Errorf("metadata = %v, want the source board and the employer", got.Metadata)
	}
}

func TestFetchOfAMissingPostingIsRecoverable(t *testing.T) {
	r := readerThatFinds(t)
	r.jobDetail = func(context.Context, string) (JobResult, error) {
		return JobResult{}, NotFoundError{What: "posting"}
	}
	res, err := connect(t, r).CallTool(context.Background(), &mcp.CallToolParams{
		Name: toolFetch, Arguments: map[string]any{"id": "gone"},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !res.IsError {
		t.Error("a missing posting answered as a success")
	}
}

package ojcpmcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/api/ojcp"
)

// jobLocation is a Place OR an array of them on the wire, while its Go type is a slice. The
// SDK validates every tool result against a schema it infers from the Go type, and inferred
// from a slice that schema admits only an array — so a single place, written as an object,
// failed every search that returned one. The projection's own tests could not see it: they
// validate against OJCP's schema, which admits both. Only a call through the SDK can.
func TestBothShapesOfJobLocationPassTheSDKsOutputValidation(t *testing.T) {
	onePlace := ojcp.JobPosting{
		OJCPID: "freehire:one", Title: "One place",
		JobLocation: ojcp.Places{{Type: "Place", Address: ojcp.Address{AddressCountry: "DE"}}},
	}
	severalPlaces := ojcp.JobPosting{
		OJCPID: "freehire:several", Title: "Several places",
		JobLocation: ojcp.Places{
			{Type: "Place", Address: ojcp.Address{AddressCountry: "DE"}},
			{Type: "Place", Address: ojcp.Address{AddressCountry: "PL"}},
		},
	}
	session := connect(t, &placesReader{jobs: []ojcp.JobPosting{onePlace, severalPlaces}})
	ctx := context.Background()

	calls := []*mcp.CallToolParams{
		{Name: toolSearchJobs, Arguments: map[string]any{"query": "go"}},
		{Name: toolGetJobDetail, Arguments: map[string]any{"job_id": "freehire:one"}},
	}
	for _, call := range calls {
		res, err := session.CallTool(ctx, call)
		if err != nil {
			t.Errorf("%s: %v", call.Name, err)
			continue
		}
		if res.IsError {
			t.Errorf("%s returned a tool error: %+v", call.Name, res.Content)
		}
	}
}

// connect drives the whole server through the SDK's in-memory transport, the way an agent
// does minus the network, so the SDK's own input and output validation runs.
func connect(t *testing.T, r Reader) *mcp.ClientSession {
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

// placesReader serves fixed postings: all of them to a search, the first to a detail read.
type placesReader struct{ jobs []ojcp.JobPosting }

func (r *placesReader) SearchJobs(context.Context, ojcp.SearchInput) (ojcp.SearchJobsResponse, error) {
	return ojcp.SearchJobsResponse{Jobs: r.jobs}.Finalize(), nil
}

func (r *placesReader) JobDetail(context.Context, string) (ojcp.JobDetailResponse, error) {
	return ojcp.JobDetailResponse{Job: r.jobs[0]}.Finalize(), nil
}

func (r *placesReader) EmployerContext(context.Context, string) (ojcp.EmployerContextResponse, error) {
	return ojcp.EmployerContextResponse{}.Finalize(), nil
}

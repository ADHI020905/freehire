package mcpapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Prompts are templates a person picks in their client (Claude lists them as commands), not
// something the model calls. Each renders one plain user request; the server instructions,
// not the prompt text, are what tell the model which tools answer it.
const (
	promptJobsForStack   = "jobs_for_my_stack"
	promptCheckShortlist = "check_my_shortlist"
	promptSalaryCheck    = "salary_check"
)

// promptSpec is one prompt and how it renders. render is handed only the arguments the
// client sent, with required ones already checked present.
type promptSpec struct {
	prompt *mcp.Prompt
	render func(args map[string]string) string
}

var prompts = []promptSpec{
	{
		prompt: &mcp.Prompt{
			Name:        promptJobsForStack,
			Title:       "Jobs for my stack",
			Description: "Open tech jobs that use your stack, optionally by seniority and location.",
			Arguments: []*mcp.PromptArgument{
				{Name: "stack", Description: "technologies, e.g. Go and Kubernetes", Required: true},
				{Name: "seniority", Description: "e.g. senior"},
				{Name: "location", Description: "e.g. remote from LATAM, or Berlin"},
			},
		},
		render: func(a map[string]string) string {
			return "Find open " + spaced(a["seniority"]) + "roles that use " + a["stack"] +
				inPhrase(a["location"]) + ". Show the ten best matches with company, location, " +
				"salary where stated, the job board each came from and the employer's own link, " +
				"and flag any that look stale or possibly not being filled."
		},
	},
	{
		prompt: &mcp.Prompt{
			Name:        promptCheckShortlist,
			Title:       "Check my shortlist",
			Description: "Whether the postings you saved are still open before you apply.",
			Arguments: []*mcp.PromptArgument{
				{Name: "postings", Description: "the postings' freehire slugs or links, one per line", Required: true},
			},
		},
		render: func(a map[string]string) string {
			return "Check whether these job postings are still open:\n" + a["postings"] +
				"\nFor each, say whether it is open or closed, and for the open ones whether it " +
				"still looks live and when it was last seen."
		},
	},
	{
		prompt: &mcp.Prompt{
			Name:        promptSalaryCheck,
			Title:       "Salary check",
			Description: "What a role pays, from open postings that state a salary.",
			Arguments: []*mcp.PromptArgument{
				{Name: "role", Description: "role family, e.g. backend, data engineering", Required: true},
				{Name: "seniority", Description: "e.g. senior"},
				{Name: "country", Description: "e.g. Germany"},
			},
		},
		render: func(a map[string]string) string {
			return "What does a " + spaced(a["seniority"]) + a["role"] + " role pay" +
				inPhrase(a["country"]) + "? Give the median and the 25th to 75th percentile " +
				"range with the sample size behind them, keep currencies separate, and say how " +
				"many matching postings are open right now."
		},
	},
}

// addPrompts registers every prompt on the server.
func addPrompts(server *mcp.Server) {
	for _, spec := range prompts {
		server.AddPrompt(spec.prompt, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			args := map[string]string{}
			for _, arg := range spec.prompt.Arguments {
				value := strings.TrimSpace(req.Params.Arguments[arg.Name])
				if arg.Required && value == "" {
					return nil, fmt.Errorf("%s needs %s", spec.prompt.Name, arg.Name)
				}
				args[arg.Name] = value
			}
			return &mcp.GetPromptResult{
				Description: spec.prompt.Description,
				Messages: []*mcp.PromptMessage{{
					Role:    "user",
					Content: &mcp.TextContent{Text: spec.render(args)},
				}},
			}, nil
		})
	}
}

// spaced is an optional word followed by a space, or nothing — so an omitted argument leaves
// no double space behind.
func spaced(word string) string {
	if word == "" {
		return ""
	}
	return word + " "
}

// inPhrase is " in <place>", or nothing.
func inPhrase(place string) string {
	if place == "" {
		return ""
	}
	return " in " + place
}

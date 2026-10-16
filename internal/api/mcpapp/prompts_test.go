package mcpapp

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func promptText(t *testing.T, session *mcp.ClientSession, name string, args map[string]string) (string, error) {
	t.Helper()
	res, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}
	if len(res.Messages) != 1 || res.Messages[0].Role != "user" {
		t.Fatalf("%s: messages = %+v, want one user message", name, res.Messages)
	}
	text, ok := res.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s: content is %T, want text", name, res.Messages[0].Content)
	}
	return text.Text, nil
}

func TestEveryPromptIsListedWithItsArguments(t *testing.T) {
	listed, err := connect(t, readerThatFinds(t)).ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("prompts/list: %v", err)
	}
	want := map[string]bool{promptJobsForStack: true, promptCheckShortlist: true, promptSalaryCheck: true}
	if len(listed.Prompts) != len(want) {
		t.Fatalf("listed %d prompts, want %d", len(listed.Prompts), len(want))
	}
	for _, p := range listed.Prompts {
		if !want[p.Name] {
			t.Errorf("unexpected prompt %q", p.Name)
		}
		if p.Title == "" || p.Description == "" || len(p.Arguments) == 0 {
			t.Errorf("%s has no title, description or arguments; a client has nothing to show", p.Name)
		}
	}
}

func TestAPromptFillsInWhatItWasGivenAndLeavesOutWhatItWasNot(t *testing.T) {
	session := connect(t, readerThatFinds(t))

	full, err := promptText(t, session, promptJobsForStack, map[string]string{
		"stack": "Go and Kubernetes", "location": "remote from LATAM", "seniority": "senior",
	})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	for _, part := range []string{"senior", "Go and Kubernetes", "remote from LATAM"} {
		if !strings.Contains(full, part) {
			t.Errorf("prompt %q does not carry %q", full, part)
		}
	}

	bare, err := promptText(t, session, promptJobsForStack, map[string]string{"stack": "Rust"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.Contains(bare, "  ") || strings.Contains(bare, " in .") || strings.Contains(bare, "{") {
		t.Errorf("an omitted optional argument left a trace: %q", bare)
	}
}

func TestAPromptWithoutItsRequiredArgumentIsRefused(t *testing.T) {
	session := connect(t, readerThatFinds(t))
	for _, name := range []string{promptJobsForStack, promptCheckShortlist, promptSalaryCheck} {
		if _, err := promptText(t, session, name, map[string]string{}); err == nil {
			t.Errorf("%s rendered with its required argument missing", name)
		}
	}
}

func TestTheInstructionsIntroduceEveryTool(t *testing.T) {
	// The instructions are where cross-tool guidance belongs — tool descriptions may not
	// carry it. A tool they never mention is one the model has to discover by accident.
	// search and fetch are left out on purpose: they exist for the deep-research connectors,
	// which call them by name without being told.
	for _, name := range []string{toolSearchJobs, toolGetJob, toolSearchCompanies, toolGetCompany,
		toolCheckJobsOpen, toolSimilarJobs, toolSalaryStats, toolMarketStats} {
		if !strings.Contains(serverInstructions, name) {
			t.Errorf("the instructions never mention %s", name)
		}
	}
}

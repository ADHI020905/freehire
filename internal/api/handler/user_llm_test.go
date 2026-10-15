package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// testTemplateKey names the virtual key these fakes serve a provider policy from.
const testTemplateKey = "vk-freehire-service"

// testGatewayConfig is the admin configuration every handler test uses. Spelling it once
// keeps a new required field from having to be added at each call site.
func testGatewayConfig(url string) llmkey.Config {
	return llmkey.Config{
		BaseURL: url, AdminUsername: "admin", AdminPassword: "secret", TemplateKey: testTemplateKey,
	}
}

// stubKeyQueries is the smallest store the resolver needs, for the handler-specific tests
// below (llmkey.Bind's own behavior, including the claim/clear paths, is tested in
// internal/ai/llmkey/bind_test.go — this stub only has to compile against llmkey.Queries).
type stubKeyQueries struct {
	mu        sync.Mutex
	stored    map[int64]string
	storedIDs map[int64]string
}

func newStubKeyQueries() *stubKeyQueries {
	return &stubKeyQueries{stored: map[int64]string{}, storedIDs: map[int64]string{}}
}

func (s *stubKeyQueries) GetUserLLMKey(_ context.Context, id int64) (db.GetUserLLMKeyRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return db.GetUserLLMKeyRow{LlmKey: s.stored[id], LlmKeyID: s.storedIDs[id]}, nil
}

func (s *stubKeyQueries) ClaimUserLLMKey(_ context.Context, arg db.ClaimUserLLMKeyParams) (db.ClaimUserLLMKeyRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stored[arg.ID] = arg.LlmKey.String
	s.storedIDs[arg.ID] = arg.LlmKeyID.String
	return db.ClaimUserLLMKeyRow{LlmKey: arg.LlmKey.String, LlmKeyID: arg.LlmKeyID.String}, nil
}

func (s *stubKeyQueries) ClearUserLLMKey(_ context.Context, arg db.ClearUserLLMKeyParams) error {
	delete(s.stored, arg.ID)
	delete(s.storedIDs, arg.ID)
	return nil
}

// The typed-nil trap, caught in production by the integration suite and not by a unit test
// that passed an untyped nil. llmkey.Bind returns *llm.Client; assigning a nil one into the
// runner's Model INTERFACE produces a non-nil interface holding a nil pointer, and the
// runner then dereferences it on its first round — inside the stream goroutine, after the
// response has begun.
func TestBoundRunnerKeepsTheOriginalWhenNoClientResolves(t *testing.T) {
	original := assistant.NewRunner(&nilSafeModel{}, assistant.NewStore(nil), assistant.RunnerConfig{MaxSteps: 3})
	h := &assistantHandlers{
		runner: original,
		keys:   llmkey.NewResolver(newStubKeyQueries(), nil),
		// llm is nil: this deployment has no assistant model client to re-credential.
	}

	got, err := h.boundRunner(plan.WithPaidAI(context.Background()), assistant.Session{UserID: 7, Preset: assistant.PresetChat})
	if err != nil {
		t.Fatalf("boundRunner on a paid context: %v", err)
	}
	if got != original {
		t.Error("an unresolved client replaced the runner's model with a typed nil; the turn would panic mid-stream")
	}
}

// The structural half of the paid gate: a model client is handed out only for a context the
// paid-AI middleware marked. A route that makes a model call and was mounted without the
// middleware must fail here — on its first request and in its first test — rather than
// spend on somebody who never paid.
func TestTheBindingFailsClosedWithoutThePaidMark(t *testing.T) {
	client, _, err := llm.NewClient(llm.Settings{BaseURL: "http://127.0.0.1:1", APIKey: "k", Model: "m"}, "test")
	if err != nil {
		t.Fatalf("llm client: %v", err)
	}
	b := llmBinding{client: client, keys: llmkey.NewResolver(newStubKeyQueries(), nil)}
	unpaid := context.Background()

	if got, err := b.bind(unpaid, 7, llm.Feature(tagSearchIntent)); !errors.Is(err, errUnpaidModelCall) || got != nil {
		t.Errorf("bind on an unmarked context = (%v, %v), want (nil, errUnpaidModelCall)", got, err)
	}
	if _, err := b.caller(unpaid, 7, llm.Feature(tagATSReview)); !errors.Is(err, errUnpaidModelCall) {
		t.Errorf("caller on an unmarked context error = %v, want errUnpaidModelCall", err)
	}
	h := &assistantHandlers{llm: client, keys: b.keys}
	if _, err := h.boundRunner(unpaid, assistant.Session{UserID: 7, Preset: assistant.PresetChat}); !errors.Is(err, errUnpaidModelCall) {
		t.Errorf("boundRunner on an unmarked context error = %v, want errUnpaidModelCall", err)
	}

	paid := plan.WithPaidAI(unpaid)
	if got, err := b.bind(paid, 7, llm.Feature(tagSearchIntent)); err != nil || got == nil {
		t.Errorf("bind on a paid context = (%v, %v), want a client", got, err)
	}
	if _, err := b.caller(paid, 7, llm.Feature(tagATSReview)); err != nil {
		t.Errorf("caller on a paid context: %v", err)
	}
}

// Every user-attributed model call in this package must go through the binding, because the
// binding is where the paid check lives. llmkey's own Bind and Caller cannot fail and check
// nothing, so a handler that reached for them directly would spend without anybody having
// asked whether the caller paid. user_llm.go is the one file allowed to.
func TestOnlyTheBindingReachesLLMKey(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == "user_llm.go" || strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range []string{"llmkey.Bind(", "llmkey.Caller("} {
			if strings.Contains(string(src), call) {
				t.Errorf("%s calls %s directly; spend through llmBinding.bind/caller so the paid check runs", f, call)
			}
		}
	}
}

// nilSafeModel stands in for the assistant's model in a runner nobody is going to run.
type nilSafeModel struct{}

func (nilSafeModel) Chat(context.Context, []llms.MessageContent, []llms.Tool, llm.ChatStream) (*llms.ContentChoice, error) {
	return &llms.ContentChoice{Content: "unused"}, nil
}

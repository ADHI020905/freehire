package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// LLM holds the provider-agnostic connection triple, an optional sampling temperature,
// and the optional Langfuse tracing triple.
//
// It exists because multiple entrypoints read the same LLM env keys — the server's Settings and the
// enrichment worker's Enrich — and two workers that are neither loaded the ENRICHMENT config
// purely to reach them, inheriting ENRICH_* validation they have no use for. What differs between
// those callers is POLICY, not shape: the server degrades when the LLM is unconfigured, a worker
// that exists to spend LLM calls must fail fast. So the values are shared and the policy is the
// caller's, expressed by whether it calls Require.
//
// llm.Settings is deliberately NOT folded in here: it is the env-free shape the library takes, so
// internal/platform/llm depends on no configuration package. Settings() is the one mapping between them.
type LLM struct {
	BaseURL string
	APIKey  string
	Model   string
	// Temperature is LLM_TEMPERATURE. Nil leaves langchaingo's own behaviour, which sends
	// "temperature":0 for a non-reasoning model id rather than omitting the field; a value
	// applies to every client built from this config in the process.
	Temperature *float64

	// StrictSchema says whether the model behind this gateway honours
	// `response_format: json_schema`. Default true, because that is the stronger
	// request and every model this ran against until now understood it.
	//
	// It is a deployment fact, not a vendor name: the same reason no model is
	// hard-coded here. Measured on 2026-08-31 against z.ai's glm-4.7-flash, which
	// answers a strict schema with a fenced array of invented objects and answers
	// `json_object` — the same prompt, one weaker request — with the exact shape
	// asked for, three times out of three. Asking for less got more.
	//
	// Turning it off does not remove the contract: every caller here already
	// spells its shape out in the prompt, and every caller validates what comes
	// back. See internal/ai/enrich/langchain.go's buildSystemPrompt, which lists
	// every field and every allowed enum value before any schema is attached.
	StrictSchema bool

	// Langfuse tracing is optional observability. All three empty simply means tracing is off
	// and the caller runs unchanged; a partial configuration is treated as off, mirroring how a
	// missing MEILI_MASTER_KEY disables search.
	LangfuseBaseURL   string
	LangfusePublicKey string
	LangfuseSecretKey string
}

// LoadLLM reads the LLM settings permissively. Nothing is required here — see Require.
func LoadLLM() LLM {
	return LLM{
		BaseURL:           os.Getenv("LLM_BASE_URL"),
		APIKey:            os.Getenv("LLM_API_KEY"),
		Model:             os.Getenv("LLM_MODEL"),
		Temperature:       envOptionalFloat("LLM_TEMPERATURE"),
		StrictSchema:      envBool("LLM_STRICT_SCHEMA", true),
		LangfuseBaseURL:   os.Getenv("LANGFUSE_BASE_URL"),
		LangfusePublicKey: os.Getenv("LANGFUSE_PUBLIC_KEY"),
		LangfuseSecretKey: os.Getenv("LANGFUSE_SECRET_KEY"),
	}
}

// Require reports every missing connection setting at once, for the callers whose whole job is
// to spend LLM calls: a worker that starts without a model enriches nothing and says nothing.
// Naming all three in one error means one run fixes the configuration rather than three.
//
// Tracing is never required — it is observability, not capability.
func (l LLM) Require() error {
	var missing []string
	for _, v := range []struct {
		key, value string
	}{
		{"LLM_BASE_URL", l.BaseURL},
		{"LLM_API_KEY", l.APIKey},
		{"LLM_MODEL", l.Model},
	} {
		if v.value == "" {
			missing = append(missing, v.key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: missing required env: %s", strings.Join(missing, ", "))
	}
	if l.Temperature != nil && (math.IsNaN(*l.Temperature) || math.IsInf(*l.Temperature, 0) || *l.Temperature < 0 || *l.Temperature > 2) {
		return fmt.Errorf("config: LLM_TEMPERATURE must be a number between 0 and 2")
	}
	return nil
}

// Settings maps onto the shape internal/platform/llm takes. It is the only place the two vocabularies
// meet, so a field added to either is a one-line change here rather than seven copies at the
// entrypoints.
func (l LLM) Settings(model string) llm.Settings {
	return llm.Settings{
		BaseURL:           l.BaseURL,
		APIKey:            l.APIKey,
		Model:             model,
		Temperature:       l.Temperature,
		SchemaMode:        schemaMode(l.StrictSchema),
		LangfuseBaseURL:   l.LangfuseBaseURL,
		LangfusePublicKey: l.LangfusePublicKey,
		LangfuseSecretKey: l.LangfuseSecretKey,
	}
}

// envOptionalFloat distinguishes an unset sampling parameter from an explicit zero.
// A malformed value is carried as NaN so Require/NewClient can reject it instead of
// silently falling back to the library's default.
func envOptionalFloat(key string) *float64 {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		value = math.NaN()
	}
	return &value
}

// schemaMode turns the env's yes/no into the library's vocabulary. The two are
// deliberately different words: the deployment answers "does this gateway honour a
// strict schema", while the library takes "how should a schema be asked for" — and
// the second has room for an answer the first does not anticipate.
func schemaMode(strict bool) llm.SchemaMode {
	if strict {
		return llm.SchemaModeStrict
	}

	return llm.SchemaModeJSONObject
}

package ojcp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/gowebpki/jcs"
)

var signedAt = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func testSigner(t *testing.T) *Signer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	signer, err := NewSigner(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), "freehire.me")
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	return signer
}

// verifyAsAnAgent runs the spec's Validation Procedure against the published key set, the
// way an agent that holds nothing but the two documents would. It reports the first step
// that fails, or "" when the manifest verifies.
func verifyAsAnAgent(t *testing.T, manifestJSON, keySetJSON []byte) string {
	t.Helper()
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(manifestJSON, &doc); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}
	var sig struct {
		Alg, Kid, Iss, Value string
		SignedAt             string `json:"signed_at"`
		ExpiresAt            string `json:"expires_at"`
	}
	if err := json.Unmarshal(doc["signature"], &sig); err != nil {
		return "no signature"
	}
	delete(doc, "signature")
	stripped, _ := json.Marshal(doc)
	canonical, err := jcs.Transform(stripped)
	if err != nil {
		t.Fatalf("jcs: %v", err)
	}

	var set struct {
		Keys []struct{ Kty, Crv, X, Y, Kid, Alg string }
	}
	if err := json.Unmarshal(keySetJSON, &set); err != nil {
		t.Fatalf("key set is not JSON: %v", err)
	}
	for _, k := range set.Keys {
		if k.Kid != sig.Kid {
			continue
		}
		if k.Alg != sig.Alg {
			return "algorithm_mismatch"
		}
		x, _ := base64.RawURLEncoding.DecodeString(k.X)
		y, _ := base64.RawURLEncoding.DecodeString(k.Y)
		pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
		if err != nil {
			return "key_resolution_failed"
		}
		raw, err := base64.RawURLEncoding.DecodeString(sig.Value)
		if err != nil || len(raw) != 64 {
			return "invalid_signature"
		}
		digest := sha256.Sum256(canonical)
		if !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(raw[:32]), new(big.Int).SetBytes(raw[32:])) {
			return "invalid_signature"
		}
		return ""
	}
	return "key_resolution_failed"
}

func TestASignedManifestVerifiesAgainstThePublishedKeySet(t *testing.T) {
	signer := testSigner(t)

	manifestJSON, _ := json.Marshal(signer.Sign(testManifest(), signedAt))
	keySetJSON, _ := json.Marshal(signer.KeySet())

	if failed := verifyAsAnAgent(t, manifestJSON, keySetJSON); failed != "" {
		t.Fatalf("an agent rejects the signed manifest at %s", failed)
	}
}

func TestATamperedManifestFailsVerification(t *testing.T) {
	signer := testSigner(t)
	signed := signer.Sign(testManifest(), signedAt)
	signed.MCPEndpoint = "https://attacker.example/mcp"

	manifestJSON, _ := json.Marshal(signed)
	keySetJSON, _ := json.Marshal(signer.KeySet())

	if failed := verifyAsAnAgent(t, manifestJSON, keySetJSON); failed != "invalid_signature" {
		t.Fatalf("a redirected mcp_endpoint verified (%q), want invalid_signature", failed)
	}
}

func TestTheSignatureStatesWhoSignedAndForHowLong(t *testing.T) {
	signer := testSigner(t)

	sig := signer.Sign(testManifest(), signedAt).Signature

	if sig == nil {
		t.Fatal("signature is nil")
		return // staticcheck cannot see that Fatal does not return
	}
	if sig.Alg != "ES256" {
		t.Errorf("alg = %q, want ES256", sig.Alg)
	}
	if sig.Iss != "freehire.me" {
		t.Errorf("iss = %q, want freehire.me — agents compare it to the serving domain", sig.Iss)
	}
	if sig.SignedAt != "2026-10-13T12:00:00Z" || sig.ExpiresAt != "2026-10-14T12:00:00Z" {
		t.Errorf("validity = %s..%s, want the 24 hours the spec recommends", sig.SignedAt, sig.ExpiresAt)
	}
	if keys := signer.KeySet().Keys; len(keys) != 1 || keys[0].Kid != sig.Kid {
		t.Errorf("kid %q does not name the one published key %+v", sig.Kid, keys)
	}
}

func TestASignedManifestConformsToTheSchema(t *testing.T) {
	signed := testSigner(t).Sign(testManifest(), signedAt)

	if err := validateAgainstSchema(t, schemaManifest, signed); err != nil {
		t.Fatalf("signed manifest rejected: %v", err)
	}
}

func TestEveryPublishedKeyCarriesWhatTheSpecRequires(t *testing.T) {
	// "Each key MUST include kid, kty, and alg."
	for _, k := range testSigner(t).KeySet().Keys {
		if k.Kid == "" || k.Kty == "" || k.Alg == "" {
			t.Errorf("key %+v lacks kid, kty or alg", k)
		}
	}
}

func TestNewSignerRefusesAKeyItCannotSignWith(t *testing.T) {
	rsaLike := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("not a key")})
	for name, input := range map[string][]byte{"empty": nil, "garbage": rsaLike} {
		if _, err := NewSigner(input, "freehire.me"); err == nil {
			t.Errorf("%s: NewSigner accepted it", name)
		}
	}
}

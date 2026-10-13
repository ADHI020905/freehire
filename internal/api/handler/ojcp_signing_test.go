package handler

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ojcp"
)

func manifestApp(t *testing.T, signer *ojcp.Signer) *fiber.App {
	t.Helper()
	h := newOJCPHandlers(&fakeSearcher{}, fakeOJCPStore{}, "https://freehire.me", nil)
	h.signer = signer
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/ojcp/manifest", h.OJCPManifest)
	app.Get("/ojcp/keys", h.OJCPKeys)
	return app
}

func newTestSigner(t *testing.T) *ojcp.Signer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	signer, err := ojcp.NewSigner(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), "freehire.me")
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	return signer
}

func TestASigningDeploymentSignsTheManifestAndPublishesItsKey(t *testing.T) {
	app := manifestApp(t, newTestSigner(t))

	_, manifest := doGet(t, app, "/ojcp/manifest")
	if _, ok := manifest["signature"].(map[string]any); !ok {
		t.Fatalf("manifest carries no signature: %v", manifest)
	}

	req := httptest.NewRequestWithContext(t.Context(), fiber.MethodGet, "/ojcp/keys", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("keys status = %d, want 200", resp.StatusCode)
	}
	// "The JWK Set MUST be served ... with Content-Type: application/jwk-set+json."
	if got := resp.Header.Get(fiber.HeaderContentType); got != "application/jwk-set+json" {
		t.Errorf("content-type = %q, want application/jwk-set+json", got)
	}
}

func TestADeploymentWithoutAKeyPublishesNeither(t *testing.T) {
	// No key is the shipped default and the rollback: the manifest stays exactly as it was,
	// and the key set does not exist rather than existing empty — an empty set beside an
	// unsigned manifest would read as a provider whose keys were all revoked.
	app := manifestApp(t, nil)

	_, manifest := doGet(t, app, "/ojcp/manifest")
	if _, ok := manifest["signature"]; ok {
		t.Errorf("unsigned deployment published a signature: %v", manifest["signature"])
	}
	if status, _ := doGet(t, app, "/ojcp/keys"); status != fiber.StatusNotFound {
		t.Errorf("keys status = %d, want 404 with no signing key", status)
	}
}

package auth

import (
	"encoding/base64"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// PresentedAPIKey returns the freehire API key a request carries as its Bearer credential, and
// whether it carries one at all. A Bearer session JWT (the extension's transport) is not a key.
// It authenticates nothing: it only names the token, for a caller that resolves it on its own
// terms — the public-read rate limiter, which must not pay a lookup for a request with none.
func PresentedAPIKey(c *fiber.Ctx) (string, bool) {
	token := bearerToken(c)
	return token, strings.HasPrefix(token, apiKeyPrefix)
}

// apiKeyLen is the length of every key GenerateAPIKey mints: the prefix and 32 random bytes
// in unpadded base64url.
var apiKeyLen = len(apiKeyPrefix) + base64.RawURLEncoding.EncodedLen(32)

// WellFormedAPIKey reports whether a token has the shape of a key we mint. Cheap and local,
// so a caller can refuse a made-up token before spending a database read on it.
func WellFormedAPIKey(token string) bool {
	if len(token) != apiKeyLen || !strings.HasPrefix(token, apiKeyPrefix) {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(token[len(apiKeyPrefix):])
	return err == nil
}

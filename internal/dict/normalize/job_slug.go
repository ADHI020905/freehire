package normalize

import (
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

// shortcodeLen is the number of base32 characters kept from the hash. 8 chars =
// 40 bits — enough that a slug collision (which also needs a matching title and
// company) is negligible; the jobs.public_slug UNIQUE constraint is the backstop.
const shortcodeLen = 8

// JobSlug builds a job's public slug: the normalized title and company joined
// with a short code derived from the dedup key (source, external_id), e.g.
// "senior-go-developer-acme-t35nijto". Empty title/company segments are dropped
// so the slug never contains an empty segment, and the short code is always
// present so the slug is never empty.
//
// The short code is a deterministic function of (source, external_id) ONLY, so
// re-ingesting the same job (an upsert of the same row) yields the same slug —
// it must not depend on volatile fields like the description.
func JobSlug(title, company, source, externalID string) string {
	segments := make([]string, 0, 3)
	if t := Slug(title); t != "" {
		segments = append(segments, t)
	}
	if c := Slug(company); c != "" {
		segments = append(segments, c)
	}
	segments = append(segments, shortcode(source, externalID))
	return strings.Join(segments, "-")
}

// shortcode hashes source and external_id into a lowercased base32 prefix. The
// NUL separator keeps ("ab","c") and ("a","bc") distinct.
func shortcode(source, externalID string) string {
	sum := sha256.Sum256([]byte(source + "\x00" + externalID))
	return strings.ToLower(base32.StdEncoding.EncodeToString(sum[:]))[:shortcodeLen]
}

// JobSlugCode returns the shortcode a job slug ends with — the part JobSlug derives
// from (source, external_id) alone, so it survives any change to the title or company
// the rest of the slug was built from. ok is false when slug does not end with a
// well-formed code, so a caller never looks up a fragment of some other string.
func JobSlugCode(slug string) (code string, ok bool) {
	if len(slug) < shortcodeLen {
		return "", false
	}
	head, code := slug[:len(slug)-shortcodeLen], slug[len(slug)-shortcodeLen:]
	if head != "" && !strings.HasSuffix(head, "-") {
		return "", false
	}
	for _, r := range code {
		if (r < 'a' || r > 'z') && (r < '2' || r > '7') {
			return "", false
		}
	}
	return code, true
}

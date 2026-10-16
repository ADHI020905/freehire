package mcpapp

import "strings"

// maxOpenChecks caps one check_jobs_open call. It bounds a single Postgres lookup and the
// answer's size; a shortlist past it is answered in part and the rest named back.
const maxOpenChecks = 50

// openCheckSlugs de-duplicates and caps a shortlist, keeping the caller's order: the slugs to
// check, and the ones past the cap to name back rather than drop.
func openCheckSlugs(in []string) (check, unchecked []string) {
	seen := make(map[string]bool, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		if len(check) < maxOpenChecks {
			check = append(check, s)
		} else {
			unchecked = append(unchecked, s)
		}
	}
	return check, unchecked
}

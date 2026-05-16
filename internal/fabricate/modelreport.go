package fabricate

import "strings"

// coAuthorPrefix is the case-insensitive git trailer key for co-author lines.
const coAuthorPrefix = "co-authored-by:"

// parseCoAuthors extracts the identities named in "Co-Authored-By: Name <email>"
// trailer lines anywhere in a commit message. Lines that are not valid
// identities are skipped.
func parseCoAuthors(message string) []Identity {
	var out []Identity
	for _, line := range strings.Split(message, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < len(coAuthorPrefix) {
			continue
		}
		if !strings.EqualFold(trimmed[:len(coAuthorPrefix)], coAuthorPrefix) {
			continue
		}
		rest := strings.TrimSpace(trimmed[len(coAuthorPrefix):])
		id, err := ParseIdentity(rest)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}

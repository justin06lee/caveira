package fabricate

import "strings"

// modelEmailExact is the set of email addresses (lowercased) known to belong to
// AI coding agents.
var modelEmailExact = map[string]bool{
	"noreply@anthropic.com": true,
	"copilot@github.com":    true,
}

// modelTokens are case-insensitive substrings that mark an identity as an AI
// coding agent when found anywhere in its name or email.
var modelTokens = []string{
	"claude", "codex", "copilot", "cursor", "aider", "devin", "opencode",
}

// IsModel reports whether an identity belongs to an AI coding agent rather than
// a human. It matches a recognized list of known agent emails, a "[bot]" name
// suffix, and a heuristic set of agent-name tokens.
func IsModel(id Identity) bool {
	name := strings.ToLower(strings.TrimSpace(id.Name))
	email := strings.ToLower(strings.TrimSpace(id.Email))
	if email != "" && modelEmailExact[email] {
		return true
	}
	if strings.HasSuffix(name, "[bot]") {
		return true
	}
	haystack := name + " " + email
	for _, tok := range modelTokens {
		if strings.Contains(haystack, tok) {
			return true
		}
	}
	return false
}

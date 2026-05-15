package fabricate

import (
	"fmt"
	"regexp"
	"strings"
)

// identityRe matches "Name <email>" with at least one '@' inside the angle brackets.
var identityRe = regexp.MustCompile(`^\s*(?P<name>.+?)\s*<\s*(?P<email>[^<>\s]+@[^<>\s]+)\s*>\s*$`)

// ParseIdentity parses a "Name <email>" string into an Identity.
func ParseIdentity(s string) (Identity, error) {
	m := identityRe.FindStringSubmatch(s)
	if m == nil {
		return Identity{}, fmt.Errorf("invalid identity %q: expected `Name <email>`", s)
	}
	name := strings.TrimSpace(m[identityRe.SubexpIndex("name")])
	email := strings.TrimSpace(m[identityRe.SubexpIndex("email")])
	if name == "" {
		return Identity{}, fmt.Errorf("invalid identity %q: name is empty", s)
	}
	return Identity{Name: name, Email: email}, nil
}

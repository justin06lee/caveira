// Package llm provides LLM provider engines and plan parsing for Caveira's
// fabricate Phase 2. It must not import the parent fabricate package.
package llm

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Change is one file's contribution to a plan commit. Exactly one of
// AllSegments / Segments is meaningful: AllSegments true means the whole file.
type Change struct {
	Path        string
	AllSegments bool
	Segments    []int
}

// PlanCommit is one commit in an LLM-authored plan.
type PlanCommit struct {
	Message string
	Type    string
	Changes []Change
}

// Plan is a full LLM-authored commit plan.
type Plan struct {
	Commits []PlanCommit
}

// wire types mirror the JSON exactly; Change needs custom segment handling.
type wirePlan struct {
	Commits []wireCommit `json:"commits"`
}

type wireCommit struct {
	Message string       `json:"message"`
	Type    string       `json:"type"`
	Changes []wireChange `json:"changes"`
}

type wireChange struct {
	Path     string          `json:"path"`
	Segments json.RawMessage `json:"segments"`
}

// ParsePlan extracts the first balanced JSON object from raw (tolerating prose
// or Markdown fences around it) and parses it into a Plan.
func ParsePlan(raw string) (*Plan, error) {
	obj, ok := extractJSONObject(raw)
	if !ok {
		return nil, errors.New("no JSON object found in LLM response")
	}
	var wp wirePlan
	if err := json.Unmarshal([]byte(obj), &wp); err != nil {
		return nil, fmt.Errorf("parse plan JSON: %w", err)
	}
	if len(wp.Commits) == 0 {
		return nil, errors.New("plan has no commits")
	}
	plan := &Plan{}
	for ci, wc := range wp.Commits {
		if wc.Message == "" {
			return nil, fmt.Errorf("commit %d has an empty message", ci)
		}
		pc := PlanCommit{Message: wc.Message, Type: wc.Type}
		for _, wch := range wc.Changes {
			if wch.Path == "" {
				return nil, fmt.Errorf("commit %d has a change with an empty path", ci)
			}
			ch := Change{Path: wch.Path}
			if err := decodeSegments(wch.Segments, &ch); err != nil {
				return nil, fmt.Errorf("commit %d change %q: %w", ci, wch.Path, err)
			}
			pc.Changes = append(pc.Changes, ch)
		}
		plan.Commits = append(plan.Commits, pc)
	}
	return plan, nil
}

// decodeSegments interprets a "segments" field that is either the string "all"
// or a JSON array of integers. A missing field defaults to "all".
func decodeSegments(raw json.RawMessage, ch *Change) error {
	if len(raw) == 0 {
		ch.AllSegments = true
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s != "all" {
			return fmt.Errorf("segments string must be \"all\", got %q", s)
		}
		ch.AllSegments = true
		return nil
	}
	var idx []int
	if err := json.Unmarshal(raw, &idx); err != nil {
		return fmt.Errorf("segments must be \"all\" or an integer array: %w", err)
	}
	ch.Segments = idx
	return nil
}

// extractJSONObject returns the first brace-balanced JSON object substring of s,
// ignoring braces inside string literals.
func extractJSONObject(s string) (string, bool) {
	start := -1
	depth := 0
	inStr := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					return s[start : i+1], true
				}
			}
		}
	}
	return "", false
}

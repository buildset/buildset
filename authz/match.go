package authz

import "strings"

// wildcard matches the rest of a value, and only at the end of a pattern.
const wildcard = "*"

// Matching happens in Go rather than SQL because GLOB, LIKE and a future in-memory backend disagree
// about escaping and case. Here the rule is identical on every backend and testable without one.
func matches(pattern, value string) bool {
	if pattern == wildcard {
		return true
	}

	if prefix, found := strings.CutSuffix(pattern, wildcard); found {
		// A prefix match only, never a substring match: "urn:content:post:*" must not reach
		// anything outside that exact prefix.
		return strings.HasPrefix(value, prefix)
	}

	return pattern == value
}

func allows(patterns []Pattern, action, resource string) bool {
	for _, pattern := range patterns {
		if matches(pattern.Action, action) && matches(pattern.Resource, resource) {
			return true
		}
	}

	return false
}

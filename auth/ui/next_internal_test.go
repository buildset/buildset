package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeNext(t *testing.T) {
	t.Parallel()

	kept := []string{
		"/",
		"/admin",
		"/admin/posts?status=draft",
		"/posts/0199bf3c-7a1e-7c2b-9f10-3d4a5b6c7d8e",
	}
	for _, raw := range kept {
		assert.Equal(t, raw, safeNext(raw), raw)
	}

	rejected := []string{
		"",
		"//evil.example",
		"///evil.example",
		`/\evil.example`,
		`/\/evil.example`,
		"https://evil.example",
		"http://evil.example/path",
		"//evil.example/path",
		"evil.example",
		"javascript:alert(1)",
		"\t//evil.example",
		"/\t/evil.example",
		"/\n/evil.example",
		"/\r\n/evil.example",
		"//user@evil.example",
	}
	for _, raw := range rejected {
		assert.Equal(t, "/", safeNext(raw), "%q must not survive", raw)
	}
}

package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeUsername(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "ada", NormalizeUsername("  Ada  "))
	assert.Equal(t, "ada_lovelace", NormalizeUsername("Ada_Lovelace"))
}

func TestValidateUsername(t *testing.T) {
	t.Parallel()

	valid := []string{"ada", "ada-lovelace", "ada_lovelace", "user42", "0ab"}
	for _, username := range valid {
		require.NoError(t, ValidateUsername(username), username)
	}

	invalid := []string{
		"",
		"ab",
		"Ada",
		"ada lovelace",
		"ada.lovelace",
		"-ada",
		"_ada",
		"ada@example.com",
		"аda", // Cyrillic a, which is why the confusable TODO exists
	}
	for _, username := range invalid {
		require.Error(t, ValidateUsername(username), username)
	}
}

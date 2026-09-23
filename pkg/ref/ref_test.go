package ref_test

import (
	"testing"

	"github.com/buildset/buildset/pkg/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()

	valid := map[string]ref.Ref{
		"urn:auth:user:0199bf3c-7a1e-7c2b-9f10-3d4a5b6c7d8e": {
			Service:      "auth",
			ResourceType: "user",
			ID:           "0199bf3c-7a1e-7c2b-9f10-3d4a5b6c7d8e",
		},
		"urn:content:post:01J8XK2P": {
			Service:      "content",
			ResourceType: "post",
			ID:           "01J8XK2P",
		},
		"urn:media:image-file:a._~-B9": {
			Service:      "media",
			ResourceType: "image-file",
			ID:           "a._~-B9",
		},
	}

	for input, want := range valid {
		got, err := ref.Parse(input)
		require.NoError(t, err, input)
		assert.Equal(t, want, got)
		assert.Equal(t, input, got.String(), "round trip")
	}

	invalid := []string{
		"",
		"urn:auth:user",
		"urn:auth:user:id:extra",
		"URN:auth:user:id",
		"uri:auth:user:id",
		"urn::user:id",
		"urn:auth::id",
		"urn:auth:user:",
		"urn:Auth:user:id",
		"urn:auth:user:id with space",
		"urn:auth:user:id/slash",
		"urn:auth:user:id%2Fescaped",
		"urn:-auth:user:id",
		"urn:9auth:user:id",
	}

	for _, input := range invalid {
		_, err := ref.Parse(input)
		require.ErrorIs(t, err, ref.ErrInvalid, input)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	r, err := ref.New("content", "post", "01J8XK2P")
	require.NoError(t, err)
	assert.Equal(t, "urn:content:post:01J8XK2P", r.String())

	_, err = ref.New("content", "post", "has:colon")
	require.ErrorIs(t, err, ref.ErrInvalid)
}

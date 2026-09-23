package authz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatches(t *testing.T) {
	t.Parallel()

	assert.True(t, matches("*", "urn:content:post:abc"))
	assert.True(t, matches("urn:content:post:*", "urn:content:post:abc"))
	assert.True(t, matches("urn:content:post:abc", "urn:content:post:abc"))

	assert.False(t, matches("urn:content:post:abc", "urn:content:post:abcd"))
	assert.False(t, matches("urn:content:post:*", "urn:auth:user:abc"))
	assert.False(t, matches("post.create", "post.created"))

	// A wildcard anywhere but the end is not special, so it cannot be used to match a substring.
	assert.False(t, matches("urn:*:post:abc", "urn:content:post:abc"))
	assert.False(t, matches("post", "post.create"))
}

func TestAllows(t *testing.T) {
	t.Parallel()

	admin := []Pattern{{Action: "*", Resource: "*"}}
	assert.True(t, allows(admin, "post.delete", "urn:content:post:abc"))

	author := []Pattern{{Action: "post.create", Resource: "urn:content:post:*"}}
	assert.True(t, allows(author, "post.create", "urn:content:post:abc"))
	assert.False(t, allows(author, "post.update", "urn:content:post:abc"))
	assert.False(t, allows(author, "post.create", "urn:auth:user:abc"))

	assert.False(t, allows(nil, "post.read", "urn:content:post:abc"))
}

func TestValidateResourceQuery(t *testing.T) {
	t.Parallel()

	valid := []string{
		"*",
		"urn:content:post:*",
		"urn:content:post:0199bf3c-7a1e-7c2b-9f10-000000000001",
		"urn:auth:user:*",
	}
	for _, resource := range valid {
		require.NoError(t, validateResourceQuery(resource), resource)
	}

	invalid := []string{
		"",
		"urn:content:*",
		"post:*",
		"urn:content:post:a b*",
		"urn:content:post:*extra",
	}
	for _, resource := range invalid {
		assert.Error(t, validateResourceQuery(resource), resource)
	}
}

package ui

import (
	"net/url"
	"strings"
)

// safeNext keeps the post-login redirect inside this site. Anything that could send the browser to
// another origin, or that a browser might normalize into doing so, collapses to the home page.
func safeNext(raw string) string {
	const fallback = "/"

	if raw == "" || !strings.HasPrefix(raw, "/") {
		return fallback
	}

	// "//host" is a protocol-relative URL and "/\host" is treated as one by several browsers.
	if strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, `/\`) {
		return fallback
	}

	for _, c := range raw {
		if c < 0x20 || c == 0x7f {
			return fallback
		}
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fallback
	}

	if parsed.Scheme != "" || parsed.Host != "" || parsed.Opaque != "" || parsed.User != nil {
		return fallback
	}

	return parsed.RequestURI()
}

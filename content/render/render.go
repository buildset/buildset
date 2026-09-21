// Package render turns a stored post body into a fragment safe to place inside a page.
//
// It is a leaf on purpose: rendering a body needs the body and its content type and nothing else,
// no repository and no service. That lets a site rendering pages against a remote content service
// render locally instead of asking for HTML over the network, while still using these exact rules.
package render

import (
	"errors"
	"fmt"
	"html/template"
	"strings"
)

// ContentTypePlainText is the only body format supported today.
const ContentTypePlainText = "text/plain"

var ErrUnsupportedContent = errors.New("unsupported content type")

// HTML turns a stored body into a fragment safe to place inside a page.
func HTML(contentType, body string) (template.HTML, error) {
	if err := ValidateContentType(contentType); err != nil {
		return "", err
	}

	return plainText(body), nil
}

// ValidateContentType reports whether a body in this format can be rendered at all.
func ValidateContentType(contentType string) error {
	if contentType != ContentTypePlainText {
		return fmt.Errorf("%w: %q", ErrUnsupportedContent, contentType)
	}

	return nil
}

// plainText escapes everything and then adds structure: a blank line starts a paragraph, a single
// newline is a line break. Nothing in the body can introduce markup.
func plainText(body string) template.HTML {
	normalized := strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")

	var builder strings.Builder

	for paragraph := range strings.SplitSeq(normalized, "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		escaped := template.HTMLEscapeString(paragraph)
		fmt.Fprintf(&builder, "<p>%s</p>", strings.ReplaceAll(escaped, "\n", "<br>"))
	}

	return template.HTML(builder.String())
}

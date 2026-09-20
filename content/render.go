package content

import (
	"fmt"
	"html/template"
	"strings"
)

// RenderHTML turns a stored body into a fragment safe to place inside a page.
//
// This lives in content rather than in the service that renders pages because the content type is
// content's own field. A second consumer, such as a feed or a search indexer, gets the same rules
// instead of writing its own copy that slowly drifts.
func RenderHTML(contentType, body string) (template.HTML, error) {
	if err := validateContentType(contentType); err != nil {
		return "", err
	}

	return renderPlainText(body), nil
}

// renderPlainText escapes everything and then adds structure: a blank line starts a paragraph, a
// single newline is a line break. Nothing in the body can introduce markup.
func renderPlainText(body string) template.HTML {
	normalized := strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")

	var builder strings.Builder

	for _, paragraph := range strings.Split(normalized, "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		escaped := template.HTMLEscapeString(paragraph)
		fmt.Fprintf(&builder, "<p>%s</p>", strings.ReplaceAll(escaped, "\n", "<br>"))
	}

	return template.HTML(builder.String())
}

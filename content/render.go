package content

import (
	"html/template"

	"github.com/buildset/buildset/content/render"
)

// RenderHTML turns a stored body into a fragment safe to place inside a page.
//
// The rules live in content/render so that a consumer holding only a body and a content type, such
// as a site rendering pages against a remote content service, applies the same ones rather than
// writing a copy that slowly drifts.
func RenderHTML(contentType, body string) (template.HTML, error) {
	return render.HTML(contentType, body)
}

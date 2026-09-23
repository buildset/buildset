package content

import (
	"html/template"

	"github.com/buildset/buildset/content/render"
)

// The rules live in content/render so a consumer holding only a body and a content type, such as a
// site running against a remote content service, applies the same ones.
func RenderHTML(contentType, body string) (template.HTML, error) {
	return render.HTML(contentType, body)
}

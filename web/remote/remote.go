// Package remote satisfies the site's dependencies by calling each service over HTTP. It mirrors
// the composition root's in-process adapters, wrapping a client rather than a service value. It
// lives here so a binary serving only the site links no service code, hashing or database driver.
package remote

import (
	"errors"

	"github.com/buildset/buildset/pkg/httpx"
	"github.com/buildset/buildset/web"
)

// translate turns a domain failure into the sentinel the site matches on and leaves everything
// else alone. That second half matters: the site treats a visitor as anonymous only when the
// identity service said so, which holds only while an unreachable one cannot yield ErrNotFound.
func translate(err error) error {
	if err == nil {
		return nil
	}

	var domain *httpx.Error
	if !errors.As(err, &domain) {
		return err
	}

	if translated := web.ErrorFromCode(domain.Code, domain.Message); translated != nil {
		return translated
	}

	return err
}

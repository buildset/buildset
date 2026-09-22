// Package remote satisfies the site's dependencies by calling each service over HTTP.
//
// It is the mirror of the composition root's in-process adapters: those wrap a service value, this
// wraps a client. web/deps.go says its interfaces use locally declared types precisely so that an
// implementation talking HTTP can satisfy them without importing the service, and this is that
// implementation.
//
// It lives beneath web rather than beside the in-process adapters so that a binary serving only
// the site links no service code, no password hashing and no database driver.
package remote

import (
	"errors"

	"github.com/buildset/buildset/pkg/httpx"
	"github.com/buildset/buildset/web"
)

// translate turns a domain failure into the sentinel the site matches on, and leaves everything
// else alone.
//
// Leaving it alone is the important half. A failure to reach a service must stay a failure: the
// site refuses to treat a visitor as anonymous unless the identity service positively said the
// session is unknown, and that only works while an unreachable service cannot produce ErrNotFound.
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

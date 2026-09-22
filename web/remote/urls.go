package remote

import (
	"net/url"

	"github.com/buildset/buildset/pkg/safeurl"
)

// URLs are the identity service's pages.
//
// They are configuration rather than calls because they are pure strings, and because rendering a
// page must not stall on a network round trip to learn where its own sign-in link points.
//
// With a gateway putting both services on one origin these stay relative, exactly as the identity
// service builds them itself, which is what keeps the session cookie and the same-origin checks
// working as they do in the single binary.
type URLs struct {
	Login    string
	Logout   string
	Password string
	Register string
}

// DefaultURLs are the paths the identity service serves.
func DefaultURLs() URLs {
	return URLs{
		Login:    "/login",
		Logout:   "/logout",
		Password: "/password",
		Register: "/register",
	}
}

// LoginURL is where an unauthenticated visitor is sent.
//
// The redirect target goes through the same check the identity service applies to it, from the
// same package, so the guarantee cannot hold on one side of the gateway and not the other.
func (u URLs) LoginURL(next string) string {
	target := safeurl.Next(next)
	if target == "/" {
		return u.Login
	}

	return u.Login + "?next=" + url.QueryEscape(target)
}

func (u URLs) LogoutURL() string   { return u.Logout }
func (u URLs) PasswordURL() string { return u.Password }
func (u URLs) RegisterURL() string { return u.Register }

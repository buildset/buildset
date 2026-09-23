package remote

import (
	"net/url"

	"github.com/buildset/buildset/pkg/safeurl"
)

// URLs are the identity service's pages. They are configuration rather than calls so rendering a
// page never stalls on a round trip, and they stay relative because the gateway puts both services
// on one origin, which is what keeps the session cookie and the same-origin checks working.
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

// The redirect target goes through the same check the identity service applies, from the same
// package, so the guarantee cannot hold on one side of the gateway and not the other.
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

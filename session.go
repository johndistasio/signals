package main

import (
	"context"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/segmentio/ksuid"
	"net/http"
	"regexp"
)

func GenerateSessionId(ctx context.Context) string {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GenerateSessionId")
	defer span.Finish()

	id, err := ksuid.NewRandom()

	if err != nil {
		ext.LogError(span, err)
		return ""
	}

	return id.String()
}

func ParseSessionId(ctx context.Context, id string) bool {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ParseSessionId")
	defer span.Finish()

	_, err := ksuid.Parse(id)

	if err != nil {
		return false
	}

	return true
}

const SessionCookieName = "id"

// SameSite represents valid values for the eponymous cookie attribute. From Mozilla:
//
// 		"The SameSite attribute lets servers require that a cookie shouldn't be sent with cross-origin requests
//		(where Site is defined by the registrable domain), which provides some protection against cross-site request
//		forgery attacks (CSRF)."
//
// This was implemented to provide a zero value of "Strict" mode since net/http's does not.
type SameSite int

const (
	// Setting a cookie in Strict mode instructs the browser to only send the cookie to the site that set it.
	SameSiteStrict SameSite = iota

	// Lax mode is similar to strict, but the browser will also send the cookie to third-party sites if they are
	// navigated to through a link on the originating site.
	SameSiteLax

	// Always send the cookie to all sites, always.
	SameSiteNone
)

var sameSiteString = [...]string{"Strict", "Lax", "None"}

func (s SameSite) String() string {
	return sameSiteString[s]
}

// Matches compares net/http's SameSite mode constants against ours.
func (s SameSite) Matches(v http.SameSite) bool {
	if s == SameSiteStrict && v == http.SameSiteStrictMode {
		return true
	}

	if s == SameSiteLax && v == http.SameSiteLaxMode {
		return true

	}

	if s == SameSiteNone && v == http.SameSiteNoneMode {
		return true
	}

	return false
}

// Convert translates our mode constant into net/http's SameSite equivalent.
func (s SameSite) Convert() http.SameSite {
	switch {
	case s == SameSiteLax:
		return http.SameSiteLaxMode
	case s == SameSiteNone:
		return http.SameSiteNoneMode
	default:
		return http.SameSiteStrictMode
	}
}

// SessionHandler is HTTP middleware that manages session cookies on incoming requests.
type SessionHandler struct {
	// CreateSessionId is a function that generates a new session ID.
	CreateSessionId func(context.Context) string

	// ValidateSessionId is a function that determines if a given session ID is valid.
	ValidateSessionId func(context.Context, string) bool

	// Generate session cookies without Secure set.
	Insecure bool

	// Generate session cookies without HttpOnly set.
	Javascript bool

	// Value for SameSite attribute of generated session cookies.
	SameSite SameSite

	// Value for the Domain attribute of generated session cookies.
	Domain string

	// Value for the Path attribute of generated session cookies.
	Path string
}

var sessionCookieRegex = regexp.MustCompile(`((^| )` + SessionCookieName + `=)[\w]+`)

func InjectSessionCookie(h http.Header, id string) {
	if h["Cookie"] == nil {
		h["Cookie"] = []string{SessionCookieName + "=" + id}
		return
	}

	for i := 0; i < len(h["Cookie"]); i++ {
		if sessionCookieRegex.MatchString(h["Cookie"][i]) {
			h["Cookie"][i] = sessionCookieRegex.ReplaceAllString(h["Cookie"][i], "${1}"+id)
			// We don't return here on the off chance that we have a client sending multiple Cookie headers.
		}
	}
}

// CreateCookie generates an *http.Cookie configured per the SessionHandler's cookie settings.
func (s *SessionHandler) CreateCookie(value string) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Domain:   s.Domain,
		Path:     s.Path,
		HttpOnly: !s.Javascript,
		Secure:   !s.Insecure,
		SameSite: s.SameSite.Convert(),
	}
}

func (s *SessionHandler) Handle(call string, next AppHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "SessionHandler.Handle")
		defer span.Finish()

		var id string

		cookie, err := r.Cookie(SessionCookieName)

		if err != nil || !s.ValidateSessionId(ctx, cookie.Value) {
			id = s.CreateSessionId(ctx)

			InjectSessionCookie(r.Header, id)
			http.SetCookie(w, s.CreateCookie(id))

		} else {
			id = cookie.Value
		}

		span.SetTag("session.id", id)

		if next != nil {
			next.Handle(id, call).ServeHTTP(w, r.WithContext(ctx))
		}
	})
}

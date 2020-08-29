package main

import (
	"context"
	"errors"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/segmentio/ksuid"
	"net/http"
	"regexp"
	"time"
)

var ErrSessionBackend = errors.New("session backend gone")

var ErrSessionExists = errors.New("duplicate session ID")

var ErrSessionId = errors.New("failed to generate new session ID")

var ErrSessionInvalid = errors.New("invalid session ID")

var ErrSessionUnknown = errors.New("unknown session ID")

type OldSession interface {
	ID() string
	Create(ctx context.Context) error
	Renew(ctx context.Context) error
	Expire(ctx context.Context) error
}

func GenerateSessionId(ctx context.Context) (string, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GenerateSessionId")
	defer span.Finish()

	id, err := ksuid.NewRandom()

	if err != nil {
		ext.LogError(span, err)
		return "", ErrSessionId
	}

	return id.String(), nil
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

type SessionStore interface {
	Create(ctx context.Context, d time.Duration) (string, error)
	Read(ctx context.Context, id string) (time.Duration, error)
	Update(ctx context.Context, id string, d time.Duration) error
	Delete(ctx context.Context, id string) error
}

const SessionCookieName = "id"

var SessionCookieRegex = regexp.MustCompile(SessionCookieName + `=[\w]+`)

type SameSite int

const (
	Strict SameSite = iota
	Lax
	None
)

func (s SameSite) String() string {
	return []string{"Strict", "Lax", "None"}[s]
}

// Matches compares net/http's SameSite mode constants against ours.
func (s SameSite) Matches(v http.SameSite) bool {
	if s == Strict && v == http.SameSiteStrictMode {
		return true
	}

	if s == Lax && v == http.SameSiteLaxMode {
		return true

	}

	if s == None && v == http.SameSiteNoneMode {
		return true
	}

	return false
}

// Matches compares net/http's SameSite mode constants against ours.
func (s SameSite) Stdlib() http.SameSite {
	switch {
	case s == Lax:
		return http.SameSiteLaxMode
	case s == None:
		return http.SameSiteNoneMode
	default:
		return http.SameSiteStrictMode
	}
}

type SessionHandler struct {
	age time.Duration

	domain string

	// Allow cookies without Secure set.
	insecure bool

	// Allow cookies without HttpOnly set.
	javascript bool

	sameSite SameSite

	path string

	store SessionStore

	next http.Handler
}

func ReplaceSessionCookie(h http.Header, id string) {
	for i := 0; i < len(h["Cookie"]); i++ {
		h["Cookie"][i] = SessionCookieRegex.ReplaceAllString(h["Cookie"][i], SessionCookieName+"="+id)
	}
}

func InjectSessionCookie(h http.Header, id string) {
	h["Cookie"] = append(h["Cookie"], SessionCookieName+"="+id)
}

func (s *SessionHandler) CreateCookie(id string) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    id,
		Domain:   s.domain,
		Path:     s.path,
		HttpOnly: !s.javascript,
		Secure:   !s.insecure,
		SameSite: s.sameSite.Stdlib(),
	}
}

func (s *SessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tracer := opentracing.GlobalTracer()
	sctx, _ := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header))
	span := tracer.StartSpan("SessionHandler.ServeHTTP", ext.RPCServerOption(sctx))
	ctx := opentracing.ContextWithSpan(r.Context(), span)
	defer span.Finish()

	cookie, err := r.Cookie(SessionCookieName)

	if err != nil {
		id, err := s.store.Create(ctx, s.age)

		if err != nil {
			ext.LogError(span, err)
			http.Error(w, "session creation failure", 500)
			return
		}

		span.SetTag("session.id", id)

		InjectSessionCookie(r.Header, id)
		http.SetCookie(w, s.CreateCookie(id))

		s.next.ServeHTTP(w, r)
		return
	}

	err = s.store.Update(ctx, cookie.Value, s.age)

	if err == ErrSessionBackend {
		ext.LogError(span, err)
		http.Error(w, "session update failure", 500)
		return
	}

	if err != nil {
		id, err := s.store.Create(ctx, s.age)

		if err != nil {
			ext.LogError(span, err)
			http.Error(w, "session creation failure", 500)
			return
		}

		cookie = s.CreateCookie(id)
		ReplaceSessionCookie(r.Header, id)
		http.SetCookie(w, cookie)
	}

	span.SetTag("session.id", cookie.Value)

	s.next.ServeHTTP(w, r)
}

package main

import (
	"context"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameSite_String(t *testing.T) {
	assert.Equal(t, "Strict", SameSiteStrict.String())
	assert.Equal(t, "Lax", SameSiteLax.String())
	assert.Equal(t, "None", SameSiteNone.String())
}

func TestSameSite_Matches(t *testing.T) {
	assert.True(t, SameSiteStrict.Matches(http.SameSiteStrictMode))
	assert.True(t, SameSiteLax.Matches(http.SameSiteLaxMode))
	assert.True(t, SameSiteNone.Matches(http.SameSiteNoneMode))
}

func TestSameSite_Convert(t *testing.T) {
	assert.Equal(t, http.SameSiteStrictMode, SameSiteStrict.Convert())
	assert.Equal(t, http.SameSiteLaxMode, SameSiteLax.Convert())
	assert.Equal(t, http.SameSiteNoneMode, SameSiteNone.Convert())
}

func TestSessionHandler_CreateCookie(t *testing.T) {
	sh := &SessionMiddleware{}
	cookie := sh.CreateCookie("test")

	assert.Equal(t, SessionCookieName, cookie.Name)
	assert.Equal(t, "test", cookie.Value)
	assert.Equal(t, "", cookie.Domain)
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
	assert.True(t, cookie.Secure)
	assert.True(t, cookie.HttpOnly)
}

func TestInjectSessionCookie(t *testing.T) {
	header := map[string][]string{
		"User-Agent":      {"Go-http-client/1.1"},
		"Accept-Encoding": {"gzip"},
		"Cookie":          {"foo=bar; id=original; test=value"},
	}

	expected := "foo=bar; id=updated; test=value"
	InjectSessionCookie(header, "updated")
	assert.Equal(t, expected, header["Cookie"][0])

	header = map[string][]string{
		"User-Agent":      {"Go-http-client/1.1"},
		"Accept-Encoding": {"gzip"},
		"Cookie":          {"id=original; test=value"},
	}

	expected = "id=updated; test=value"
	InjectSessionCookie(header, "updated")
	assert.Equal(t, expected, header["Cookie"][0])

}

func TestInjectSessionCookie_NoCookie(t *testing.T) {
	header := map[string][]string{
		"User-Agent":      {"Go-http-client/1.1"},
		"Accept-Encoding": {"gzip"},
	}

	expected := "id=updated"

	InjectSessionCookie(header, "updated")
	assert.Equal(t, expected, header["Cookie"][0])
}

// Validate that SessionMiddleware will set a session cookie on incoming requests without one.
func TestSessionMiddleware_Handler(t *testing.T) {
	mw := &SessionMiddleware{
		CreateSessionId: func(context.Context) string {
			return "test"
		},
		ValidateSessionId: func(_ context.Context, id string) bool {
			return id == "test"
		},
	}

	server := httptest.NewServer(mw.Handle(nil))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)

	res, _ := (&http.Client{}).Do(req)

	assert.Condition(t, func() bool {
		for _, cookie := range res.Cookies() {
			if cookie.Name == SessionCookieName {
				if mw.ValidateSessionId(context.Background(), cookie.Value) {
					return true
				}
			}
		}

		return false
	})

}

// Validate that SessionMiddleware will not set a new session cookie on incoming requests that provide a valid session.
func TestSessionHandler_ServeHTTP_ExistingSession(t *testing.T) {
	mw := &SessionMiddleware{
		ValidateSessionId: func(_ context.Context, id string) bool {
			return id == "test"
		},
	}

	server := httptest.NewServer(mw.Handle(nil))
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)

	req.Header.Add("Cookie", SessionCookieName+"=test")

	res, _ := (&http.Client{}).Do(req)

	assert.Condition(t, func() bool {
		for _, cookie := range res.Cookies() {
			if cookie.Name == SessionCookieName {
				return false
			}
		}

		return true
	})
}

// Validate that SessionMiddleware will set a session cookie on incoming requests with an invalid session.
func TestSessionHandler_ServeHTTP_BadSession(t *testing.T) {
	mw := &SessionMiddleware{
		CreateSessionId: func(context.Context) string {
			return "test"
		},
		ValidateSessionId: func(_ context.Context, id string) bool {
			return id == "test"
		},
	}

	server := httptest.NewServer(mw.Handle(nil))
	defer server.Close()

	badCookie := "abc123"

	req, _ := http.NewRequest("GET", server.URL, nil)

	req.Header.Add("Cookie", SessionCookieName+"="+badCookie)

	res, _ := (&http.Client{}).Do(req)

	assert.Condition(t, func() bool {
		for _, cookie := range res.Cookies() {
			if cookie.Name == SessionCookieName {
				if cookie.Value != badCookie {
					if mw.ValidateSessionId(context.Background(), cookie.Value) {
						return true
					}
				}
			}
		}

		return false
	})
}

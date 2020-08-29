package main

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

func TestSameSite_String(t *testing.T) {
	assert.Equal(t, "Strict", Strict.String())
	assert.Equal(t, "Lax", Lax.String())
	assert.Equal(t, "None", None.String())
}

func TestSameSite_Matches(t *testing.T) {
	assert.True(t, Strict.Matches(http.SameSiteStrictMode))
	assert.True(t, Lax.Matches(http.SameSiteLaxMode))
	assert.True(t, None.Matches(http.SameSiteNoneMode))
}

func TestSessionHandler_CreateCookie(t *testing.T) {
	sh := &SessionHandler{}
	cookie := sh.CreateCookie("test")

	assert.Equal(t, SessionCookieName, cookie.Name)
	assert.Equal(t, "test", cookie.Value)
	assert.Equal(t, "", cookie.Domain)
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
	assert.True(t, cookie.Secure)
	assert.True(t, cookie.HttpOnly)
}
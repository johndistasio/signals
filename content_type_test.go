package main

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContentTypeHandler (t *testing.T) {
	mw := (&ContentTypeMiddleware{[]string{"application/json"}}).Handle(nil)

	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

func TestContentTypeHandler_DisallowedType(t *testing.T) {
	mw := (&ContentTypeMiddleware{[]string{"application/json"}}).Handle(nil)

	req := httptest.NewRequest("OPTIONS", "http://example.com/foo", nil)
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestContentTypeHandler_NoHeader(t *testing.T) {
	mw := (&ContentTypeMiddleware{[]string{"application/json"}}).Handle(nil)

	req := httptest.NewRequest("OPTIONS", "http://example.com/foo", nil)
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Result().StatusCode)
}

func TestContentTypeHandler_NoType(t *testing.T) {
	mw := (&ContentTypeMiddleware{[]string{}}).Handle(nil)

	req := httptest.NewRequest("OPTIONS", "http://example.com/foo", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Result().StatusCode)
}

func TestContentTypeHandler_NilType(t *testing.T) {
	mw := (&ContentTypeMiddleware{nil}).Handle(nil)

	req := httptest.NewRequest("OPTIONS", "http://example.com/foo", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Result().StatusCode)
}

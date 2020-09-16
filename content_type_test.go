package main

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContentTypeHandler (t *testing.T) {
	handler := ContentTypeHandler(NoopHandler, "application/json")

	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

func TestContentTypeHandler_DisallowedType(t *testing.T) {
	handler := ContentTypeHandler(NoopHandler, "application/json")

	req := httptest.NewRequest("OPTIONS", "http://example.com/foo", nil)
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestContentTypeHandler_NoType(t *testing.T) {
	handler := ContentTypeHandler(NoopHandler)

	req := httptest.NewRequest("OPTIONS", "http://example.com/foo", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

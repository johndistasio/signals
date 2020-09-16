package main

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

var NoopHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })

func TestCORSHandler (t *testing.T) {
	handler := CORSHandler(NoopHandler, "*", "GET")

	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "GET, OPTIONS", resp.Header.Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, SeatHeader, resp.Header.Get("Access-Control-Allow-Headers"))
	assert.Equal(t, SeatHeader, resp.Header.Get("Access-Control-Expose-Headers"))
	assert.Equal(t, "60", resp.Header.Get("Access-Control-Max-Age"))
}

func TestCORSHandler_Options(t *testing.T) {
	handler := CORSHandler(NoopHandler, "*", "GET")

	req := httptest.NewRequest("OPTIONS", "http://example.com/foo", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Result().StatusCode)
}

func TestCORSHandler_DisallowedMethod (t *testing.T) {
	handler := CORSHandler(NoopHandler, "*", "GET")

	req := httptest.NewRequest("POST", "http://example.com/foo", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Result().StatusCode)
}


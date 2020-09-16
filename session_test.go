package main

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractCallId(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com/call/one", nil)
	call := ExtractCallId(context.Background(), req)
	assert.Equal(t, "one", call)

	req, _ = http.NewRequest("GET", "http://example.com/call/two/xyc", nil)
	call = ExtractCallId(context.Background(), req)
	assert.Equal(t, "two", call)

	req, _ = http.NewRequest("GET", "http://example.com/call/", nil)
	call = ExtractCallId(context.Background(), req)
	assert.Equal(t, "", call)
}

type SessionHandlerTestSuite struct {
	suite.Suite
	*httptest.Server
	SessionHandler
}

func (suite *SessionHandlerTestSuite) SetupTest() {
	suite.SessionHandler = SessionHandler{
		CreateSessionId:   func(context.Context) string { return "test" },
		ValidateSessionId: func(context.Context, string) bool { return true },
		ExtractCallId:     func(context.Context, *http.Request) string { return "test" },
	}

	suite.Server = httptest.NewServer(suite.SessionHandler.Handle(nil))
}

func (suite *SessionHandlerTestSuite) TearDownTest() {
	suite.Server.Close()
}

func TestSessionHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(SessionHandlerTestSuite))
}

func (suite *SessionHandlerTestSuite) TestSessionHandler() {
	req, _ := http.NewRequest("GET", suite.Server.URL, nil)
	req.Header.Add(SessionHeader, "SESSION_ID")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusOK, res.StatusCode)
	suite.Equal("SESSION_ID", res.Header.Get(SessionHeader))
}

func (suite *SessionHandlerTestSuite) TestSessionHandler_MissingSession() {
	req, _ := http.NewRequest("GET", suite.Server.URL, nil)
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusOK, res.StatusCode)
	suite.Equal("test", res.Header.Get(SessionHeader))
}


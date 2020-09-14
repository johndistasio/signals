package main

import (
	"errors"
	"github.com/johndistasio/signaling/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSplitPath_Root(t *testing.T) {
	head, call, op := SplitPath("/call/abc123")
	assert.Equal(t, "call", head)
	assert.Equal(t, "abc123", call)
	assert.Equal(t, "", op)
}

func TestSplitPath_RootSlash(t *testing.T) {
	head, call, op := SplitPath("/call/abc123/")
	assert.Equal(t, "call", head)
	assert.Equal(t, "abc123", call)
	assert.Equal(t, "", op)
}

func TestSplitPath_Operation(t *testing.T) {
	head, call, op := SplitPath("/call/abc123/signal")
	assert.Equal(t, "call", head)
	assert.Equal(t, "abc123", call)
	assert.Equal(t, "signal", op)
}

func TestSplitPath_OperationSlash(t *testing.T) {
	head, call, op := SplitPath("/call/abc123/signal")
	assert.Equal(t, "call", head)
	assert.Equal(t, "abc123", call)
	assert.Equal(t, "signal", op)
}

type SeatHandlerTestSuite struct {
	suite.Suite
	mockLock      *mock.Call
	mockPublisher *mock.Call
	server        *httptest.Server
}

func (suite *SeatHandlerTestSuite) SetupTest() {
	lock := new(mocks.Semaphore)
	suite.mockLock = lock.On("Acquire", mock.Anything, "test", "test")

	pub := new(mocks.Publisher)
	suite.mockPublisher = pub.On("Publish", mock.Anything, "test", mock.Anything)

	handler := (&SeatHandler{lock: lock, pub: pub}).Handle("test", "test")

	suite.server = httptest.NewServer(handler)
}

func (suite *SeatHandlerTestSuite) TearDownTest() {
	suite.server.Close()
}

func TestSeatHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(SeatHandlerTestSuite))
}

func (suite *SeatHandlerTestSuite) TestHandle() {
	suite.mockLock.Return(true, nil)
	suite.mockPublisher.Return(nil)

	r, _ := http.Get(suite.server.URL)
	suite.Equal(http.StatusOK, r.StatusCode)
}

func (suite *SeatHandlerTestSuite) TestSeatError() {
	suite.mockLock.Return(true, errors.New("test"))
	r, _ := http.Get(suite.server.URL)
	suite.Equal(http.StatusInternalServerError, r.StatusCode)
}

func (suite *SeatHandlerTestSuite) TestSeatConflict() {
	suite.mockLock.Return(false, nil)
	r, _ := http.Get(suite.server.URL)
	suite.Equal(http.StatusConflict, r.StatusCode)
}

func (suite *SeatHandlerTestSuite) TestMethodNotAllowed() {
	suite.mockLock.Return(true, nil)

	methods := []string{"CONNECT", "DELETE", "HEAD", "OPTIONS", "PATCH", "POST", "PUT", "TRACE"}

	for _, method := range methods {
		req, _ := http.NewRequest(method, suite.server.URL, nil)
		res, _ := (&http.Client{}).Do(req)

		suite.Equal(http.StatusMethodNotAllowed, res.StatusCode)
	}
}

type SignalHandlerTestSuite struct {
	suite.Suite
	body          *strings.Reader
	mockLock      *mock.Call
	mockPublisher *mock.Call
	server        *httptest.Server
}

func (suite *SignalHandlerTestSuite) SetupTest() {
	suite.body = strings.NewReader(`{"Message": "hello"}`)

	lock := new(mocks.Semaphore)
	suite.mockLock = lock.On("Check", mock.Anything, "test", "test")

	pub := new(mocks.Publisher)
	suite.mockPublisher = pub.On("Publish", mock.Anything, "test", mock.Anything)

	handler := (&SignalHandler{lock: lock, pub: pub}).Handle("test", "test")

	suite.server = httptest.NewServer(handler)
}

func (suite *SignalHandlerTestSuite) TearDownTest() {
	suite.server.Close()
}

func TestSignalHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(SignalHandlerTestSuite))
}

func (suite *SignalHandlerTestSuite) TestHandle() {
	suite.mockLock.Return(true, nil)
	suite.mockPublisher.Return(nil)

	req, _ := http.NewRequest("POST", suite.server.URL, suite.body)
	req.Header.Add("Content-Type", "application/json")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusOK, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestSeatConflict() {
	suite.mockLock.Return(false, nil)

	req, _ := http.NewRequest("POST", suite.server.URL, suite.body)
	req.Header.Add("Content-Type", "application/json")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusConflict, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestSeatFailure() {
	suite.mockLock.Return(false, errors.New("test"))

	req, _ := http.NewRequest("POST", suite.server.URL, suite.body)
	req.Header.Add("Content-Type", "application/json")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusInternalServerError, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestBadData() {
	suite.mockLock.Return(true, nil)

	body := strings.NewReader(`{"wrong": "potato"}`)
	req, _ := http.NewRequest("POST", suite.server.URL, body)
	req.Header.Add("Content-Type", "application/json")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusBadRequest, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestMissingData() {
	suite.mockLock.Return(true, nil)
	suite.mockPublisher.Return(nil)

	req, _ := http.NewRequest("POST", suite.server.URL, nil)
	req.Header.Add("Content-Type", "application/json")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusBadRequest, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestMissingContentType() {
	suite.mockLock.Return(true, nil)

	req, _ := http.NewRequest("POST", suite.server.URL, suite.body)
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusBadRequest, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestWrongContentType() {
	suite.mockLock.Return(true, nil)

	req, _ := http.NewRequest("POST", suite.server.URL, suite.body)
	req.Header.Add("Content-Type", "application/xml")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusBadRequest, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestMethodNotAllowed() {
	suite.mockLock.Return(true, nil)

	methods := []string{"CONNECT", "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "PUT", "TRACE"}

	for _, method := range methods {
		req, _ := http.NewRequest(method, suite.server.URL, nil)
		req.Header.Add("Content-Type", "application/json")
		res, _ := (&http.Client{}).Do(req)

		suite.Equal(http.StatusMethodNotAllowed, res.StatusCode)
	}
}

func (suite *SignalHandlerTestSuite) TestPublishFailure() {
	suite.mockLock.Return(true, nil)
	suite.mockPublisher.Return(ErrPublisherGone)

	req, _ := http.NewRequest("POST", suite.server.URL, suite.body)
	req.Header.Add("Content-Type", "application/json")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusInternalServerError, res.StatusCode)
}

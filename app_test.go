package main

import (
	"context"
	"errors"
	"github.com/go-redis/redis/v8"
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
	mockLock *mock.Call
	server   *httptest.Server
}

func (suite *SeatHandlerTestSuite) SetupTest() {
	lock := new(mocks.Semaphore)
	suite.mockLock = lock.On("Acquire", mock.Anything, CallKeyPrefix+"test", "test")

	handler := (&SeatHandler{lock: lock}).Handle("test", "test")

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
	r, _ := http.Get(suite.server.URL)
	suite.Equal(http.StatusOK, r.StatusCode)
}

func (suite *SeatHandlerTestSuite) TestHandleError() {
	suite.mockLock.Return(true, errors.New("test"))
	r, _ := http.Get(suite.server.URL)
	suite.Equal(http.StatusInternalServerError, r.StatusCode)
}

func (suite *SeatHandlerTestSuite) TestHandleConflict() {
	suite.mockLock.Return(false, nil)
	r, _ := http.Get(suite.server.URL)
	suite.Equal(http.StatusConflict, r.StatusCode)
}

func (suite *SeatHandlerTestSuite) TestPOST() {
	suite.mockLock.Return(true, nil)

	req, _ := http.NewRequest("POST", suite.server.URL, nil)
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusMethodNotAllowed, res.StatusCode)
}

func (suite *SeatHandlerTestSuite) TestPUT() {
	suite.mockLock.Return(true, nil)

	req, _ := http.NewRequest("PUT", suite.server.URL, nil)
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusMethodNotAllowed, res.StatusCode)
}

func (suite *SeatHandlerTestSuite) TestDELETE() {
	suite.mockLock.Return(true, nil)

	req, _ := http.NewRequest("DELETE", suite.server.URL, nil)
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusMethodNotAllowed, res.StatusCode)
}

type SignalHandlerTestSuite struct {
	suite.Suite
	mockLock  *mock.Call
	mockRedis *mock.Call
	server    *httptest.Server
}

func (suite *SignalHandlerTestSuite) SetupTest() {
	lock := new(mocks.Semaphore)
	suite.mockLock = lock.On("Check", mock.Anything, CallKeyPrefix+"test", "test")

	rdb := new(mocks.Redis)
	suite.mockRedis = rdb.On("Publish", mock.Anything, ChannelKeyPrefix+"test", mock.Anything)

	handler := (&SignalHandler{lock: lock, redis: rdb}).Handle("test", "test")

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
	suite.mockRedis.Return(redis.NewIntCmd(context.Background(), 1))

	body := strings.NewReader(`{"Message": "hello"}`)
	req, _ := http.NewRequest("POST", suite.server.URL, body)
	req.Header.Add("Content-Type", "application/json")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusOK, res.StatusCode)
}


func (suite *SignalHandlerTestSuite) TestHandleConflict() {
	suite.mockLock.Return(false, nil)

	req, _ := http.NewRequest("POST", suite.server.URL, nil)
	req.Header.Add("Content-Type", "application/json")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusConflict, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestHandleSeatError() {
	suite.mockLock.Return(true, errors.New("test"))

	req, _ := http.NewRequest("POST", suite.server.URL, nil)
	req.Header.Add("Content-Type", "application/json")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusInternalServerError, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestHandleBadData() {
	suite.mockLock.Return(true, nil)
	suite.mockRedis.Return(redis.NewIntCmd(context.Background(), 1))

	body := strings.NewReader(`{"wrong": "potato"}`)
	req, _ := http.NewRequest("POST", suite.server.URL, body)
	req.Header.Add("Content-Type", "application/json")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusBadRequest, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestMissingContentType() {
	suite.mockLock.Return(true, nil)
	suite.mockRedis.Return(redis.NewIntCmd(context.Background(), 1))

	req, _ := http.NewRequest("POST", suite.server.URL, nil)
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusBadRequest, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestWrongContentType() {
	suite.mockLock.Return(true, nil)
	suite.mockRedis.Return(redis.NewIntCmd(context.Background(), 1))

	req, _ := http.NewRequest("POST", suite.server.URL, nil)
	req.Header.Add("Content-Type", "application/xml")
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusBadRequest, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestGET() {
	suite.mockLock.Return(true, nil)
	suite.mockRedis.Return(redis.NewIntCmd(context.Background(), 1))

	r, _ := http.Get(suite.server.URL)

	suite.Equal(http.StatusMethodNotAllowed, r.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestPUT() {
	suite.mockLock.Return(true, nil)
	suite.mockRedis.Return(redis.NewIntCmd(context.Background(), 1))

	req, _ := http.NewRequest("PUT", suite.server.URL, nil)
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusMethodNotAllowed, res.StatusCode)
}

func (suite *SignalHandlerTestSuite) TestDELETE() {
	suite.mockLock.Return(true, nil)
	suite.mockRedis.Return(redis.NewIntCmd(context.Background(), 1))

	req, _ := http.NewRequest("DELETE", suite.server.URL, nil)
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusMethodNotAllowed, res.StatusCode)
}

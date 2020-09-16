package main

import (
	"context"
	"errors"
	"github.com/johndistasio/signaling/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"net/http"
	"net/http/httptest"
	"testing"
)

type CallHandlerTestSuite struct {
	suite.Suite
	mockLock      *mock.Call
	mockPublisher *mock.Call
	server        *httptest.Server
	url           string
}

func (suite *CallHandlerTestSuite) SetupTest() {
	lock := new(mocks.Semaphore)
	suite.mockLock = lock.On("Acquire", mock.Anything, "test", mock.Anything)

	pub := new(mocks.Publisher)
	suite.mockPublisher = pub.On("Publish", mock.Anything, "test", mock.Anything)

	handler := CallHandler(lock, pub)

	suite.server = httptest.NewServer(handler)

	suite.url = suite.server.URL + "/call/test"
}

func (suite *CallHandlerTestSuite) TearDownTest() {
	suite.server.Close()
}

func TestCallHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(CallHandlerTestSuite))
}

func (suite *CallHandlerTestSuite) TestHandle() {
	suite.mockLock.Return(true, nil)
	suite.mockPublisher.Return(nil)

	r, _ := http.Get(suite.url)
	suite.Equal(http.StatusNoContent, r.StatusCode)
}

func (suite *CallHandlerTestSuite) TestHandleNotFound() {
	suite.mockLock.Return(true, nil)
	suite.mockPublisher.Return(nil)

	r, _ := http.Get(suite.server.URL)
	suite.Equal(http.StatusNotFound, r.StatusCode)
}

func (suite *CallHandlerTestSuite) TestHandleRenew() {
	suite.mockLock.Return(true, nil)
	suite.mockPublisher.Return(nil)

	req, _ := http.NewRequest("GET", suite.url, nil)

	seat := GenerateSessionId(context.Background())

	req.Header.Add(SeatHeader, seat)
	res, _ := (&http.Client{}).Do(req)

	suite.Equal(http.StatusNoContent, res.StatusCode)
	suite.Equal(seat, res.Header.Get(SeatHeader))
}

func (suite *CallHandlerTestSuite) TestSeatError() {
	suite.mockLock.Return(true, errors.New("test"))
	r, _ := http.Get(suite.url)
	suite.Equal(http.StatusInternalServerError, r.StatusCode)
}

func (suite *CallHandlerTestSuite) TestSeatConflict() {
	suite.mockLock.Return(false, nil)
	r, _ := http.Get(suite.url)
	suite.Equal(http.StatusConflict, r.StatusCode)
}

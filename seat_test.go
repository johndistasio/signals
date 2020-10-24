package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/johndistasio/signals/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"io/ioutil"
	"net/http/httptest"
	"testing"
)

type SeatHandlerTestSuite struct {
	suite.Suite
	Lock      *mocks.Semaphore
	Publisher *mocks.Publisher
	Call      string
	Session   string
	URL       string
	Handler   *SeatHandler
}

func (suite *SeatHandlerTestSuite) SetupTest() {
	suite.Lock = new(mocks.Semaphore)
	suite.Publisher = new(mocks.Publisher)
	suite.Call = "testCall"
	suite.Session = "testSession"
	suite.URL = "http://localhost/call/" + suite.Call

	suite.Handler = &SeatHandler{
		Generator: func(_ context.Context) string { return suite.Session },
		Lock:      suite.Lock,
		Publisher: suite.Publisher,
	}
}

func TestSeatHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(SeatHandlerTestSuite))
}

// Validate the behavior of a successful request.
func (suite *SeatHandlerTestSuite) TestSeatHandler_ServeHTTP() {
	bytes, _ := json.Marshal(Event{
		Call:    suite.Call,
		Session: suite.Session,
		Kind:    MessageKindPeerJoin,
	})

	suite.Lock.On("Acquire", mock.Anything, suite.Call, suite.Session).Return(true, nil)
	suite.Publisher.On("Publish", mock.Anything, suite.Call, bytes).Return(nil)

	req := httptest.NewRequest("GET", suite.URL, nil)
	w := httptest.NewRecorder()

	suite.Handler.ServeHTTP(w, req)

	resp := w.Result()

	suite.Equal(200, resp.StatusCode)
	suite.Equal("application/json", resp.Header.Get("Content-Type"))

	body, _ := ioutil.ReadAll(resp.Body)

	var event Event

	_ = json.Unmarshal(body, &event)

	suite.Equal(suite.Call, event.Call)
	suite.Equal(suite.Session, event.Session)
	suite.Equal(MessageKindJoin, event.Kind)

	suite.Lock.AssertExpectations(suite.T())
	suite.Publisher.AssertExpectations(suite.T())
}

// Validate that we return a 404 to the client when they don't provide a call in the request URL.
func (suite *SeatHandlerTestSuite) TestSeatHandler_ServeHTTP_NoCall() {
	req := httptest.NewRequest("GET", "http://localhost/call", nil)
	w := httptest.NewRecorder()

	suite.Handler.ServeHTTP(w, req)

	suite.Equal(404, w.Result().StatusCode)
}

// Validate that we return a 409 to the client when no seats are available.
func (suite *SeatHandlerTestSuite) TestSeatHandler_ServeHTTP_NoSeatAvailable() {
	suite.Lock.On("Acquire", mock.Anything, suite.Call, suite.Session).Return(
		false, nil)

	req := httptest.NewRequest("GET", suite.URL, nil)
	w := httptest.NewRecorder()

	suite.Handler.ServeHTTP(w, req)

	suite.Equal(409, w.Result().StatusCode)
	suite.Lock.AssertExpectations(suite.T())
}

// Validate that we return a 500 to the client upon a failure to attempt to obtain a seat.
func (suite *SeatHandlerTestSuite) TestSeatHandler_ServeHTTP_SeatFailure() {
	suite.Lock.On("Acquire", mock.Anything, suite.Call, suite.Session).Return(
		false, errors.New("uh oh"))

	req := httptest.NewRequest("GET", suite.URL, nil)
	w := httptest.NewRecorder()

	suite.Handler.ServeHTTP(w, req)

	suite.Equal( 500, w.Result().StatusCode)
	suite.Lock.AssertExpectations(suite.T())
}

// Validate that we attempt to release the acquired seat and return a 500 to the client upon a failure to publish
// the "new peer" message.
func (suite *SeatHandlerTestSuite) TestSeatHandler_PublisherFailure() {
	bytes, _ := json.Marshal(Event{
		Call:    suite.Call,
		Session: suite.Session,
		Kind:    MessageKindPeerJoin,
	})

	suite.Lock.On("Acquire", mock.Anything, suite.Call, suite.Session).Return(true, nil)
	suite.Lock.On("Release", mock.Anything, suite.Call, suite.Session).Return(nil)
	suite.Publisher.On("Publish", mock.Anything, suite.Call, bytes).Return(errors.New("uh oh"))

	req := httptest.NewRequest("GET", suite.URL, nil)
	w := httptest.NewRecorder()

	suite.Handler.ServeHTTP(w, req)

	resp := w.Result()

	suite.Equal(500, resp.StatusCode)
	suite.Lock.AssertExpectations(suite.T())
	suite.Publisher.AssertExpectations(suite.T())
}

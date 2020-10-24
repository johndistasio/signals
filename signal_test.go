package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/johndistasio/signals/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"net/http/httptest"
	"strings"
	"testing"
)

type SignalHandlerTestSuite struct {
	suite.Suite
	Body      string
	Call      string
	Session   string
	URL       string
	Event     Event
	Lock      *mocks.Semaphore
	Check     *mock.Call
	Publisher *mocks.Publisher
	Publish   *mock.Call
	Handler   *SignalHandler
}

func (suite *SignalHandlerTestSuite) SetupTest() {
	suite.Body = "testBody"
	suite.Call = "testCall"
	suite.Session = "testSession"
	suite.URL = "http://localhost/signal"

	suite.Lock = new(mocks.Semaphore)
	suite.Check = suite.Lock.On(
		"Check", mock.Anything, suite.Call, suite.Session).Return(true, nil)

	suite.Event = Event{
		Body:    suite.Body,
		Call:    suite.Call,
		Kind:    MessageKindOffer,
		Session: suite.Session,
	}

	encoded, _ := json.Marshal(suite.Event)

	suite.Publisher = new(mocks.Publisher)
	suite.Publish = suite.Publisher.On(
		"Publish", mock.Anything, suite.Call, encoded).Return(nil)

	suite.Handler = &SignalHandler{
		Lock:      suite.Lock,
		Publisher: suite.Publisher,
		MaxRead:   512,
	}
}

func TestSignalHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(SignalHandlerTestSuite))
}

func (suite *SignalHandlerTestSuite) TestSignalHandler_ServeHTTP() {
	encoded, _ := json.Marshal(suite.Event)

	req := httptest.NewRequest("POST", suite.URL, bytes.NewReader(encoded))
	w := httptest.NewRecorder()

	suite.Handler.ServeHTTP(w, req)

	suite.Equal(200, w.Result().StatusCode)
	suite.Lock.AssertExpectations(suite.T())
	suite.Publisher.AssertExpectations(suite.T())
}

func (suite *SignalHandlerTestSuite) TestSignalHandler_ServeHTTP_EmptyBody() {
	req := httptest.NewRequest("POST", suite.URL, nil)
	w := httptest.NewRecorder()

	suite.Handler.ServeHTTP(w, req)

	suite.Equal(400, w.Result().StatusCode)
	suite.Lock.AssertNotCalled(suite.T(), "Lock", mock.Anything, mock.Anything, mock.Anything)
	suite.Publisher.AssertNotCalled(suite.T(), "Publish", mock.Anything, mock.Anything)
}

func (suite *SignalHandlerTestSuite) TestSignalHandler_ServeHTTP_BodyTooLarge() {
	req := httptest.NewRequest("POST", suite.URL, strings.NewReader(suite.Body))
	w := httptest.NewRecorder()

	suite.Handler.MaxRead = 1

	suite.Handler.ServeHTTP(w, req)

	suite.Equal(400, w.Result().StatusCode)
	suite.Lock.AssertNotCalled(suite.T(), "Lock", mock.Anything, mock.Anything, mock.Anything)
	suite.Publisher.AssertNotCalled(suite.T(), "Publish", mock.Anything, mock.Anything)
}

func (suite *SignalHandlerTestSuite) TestSignalHandler_ServeHTTP_BadRequest() {
	tests := map[Event]int{
		Event{
			Body:    "",
			Call:    suite.Call,
			Kind:    MessageKindOffer,
			Session: suite.Session,
		}: 400,
		Event{
			Body:    suite.Body,
			Call:    "",
			Kind:    MessageKindOffer,
			Session: suite.Session,
		}: 400,
		Event{
			Body:    suite.Body,
			Call:    suite.Call,
			Kind:    "",
			Session: suite.Session,
		}: 400,
		Event{
			Body:    suite.Body,
			Call:    suite.Call,
			Kind:    MessageKindOffer,
			Session: "",
		}: 400,
	}

	for event, code := range tests {
		encoded, _ := json.Marshal(event)

		req := httptest.NewRequest("POST", suite.URL, bytes.NewReader(encoded))
		w := httptest.NewRecorder()

		suite.Handler.ServeHTTP(w, req)

		suite.Equal(code, w.Result().StatusCode)
		suite.Lock.AssertNotCalled(suite.T(), "Check", mock.Anything, mock.Anything, mock.Anything)
		suite.Publisher.AssertNotCalled(suite.T(), "Publish", mock.Anything, mock.Anything)
	}
}

func (suite *SignalHandlerTestSuite) TestSignalHandler_ServeHTTP_SeatFailure() {
	encoded, _ := json.Marshal(suite.Event)
	req := httptest.NewRequest("POST", suite.URL, bytes.NewReader(encoded))
	w := httptest.NewRecorder()

	suite.Check.Return(false, errors.New("yikes"))

	suite.Handler.ServeHTTP(w, req)

	suite.Equal(500, w.Result().StatusCode)
	suite.Lock.AssertExpectations(suite.T())
	suite.Publisher.AssertNotCalled(suite.T(), "Publish", mock.Anything, mock.Anything)
}

func (suite *SignalHandlerTestSuite) TestSignalHandler_ServeHTTP_SeatNotHeld() {
	encoded, _ := json.Marshal(suite.Event)
	req := httptest.NewRequest("POST", suite.URL, bytes.NewReader(encoded))
	w := httptest.NewRecorder()

	suite.Check.Return(false, nil)

	suite.Handler.ServeHTTP(w, req)

	suite.Equal(409, w.Result().StatusCode)
	suite.Lock.AssertExpectations(suite.T())
	suite.Publisher.AssertNotCalled(suite.T(), "Publish", mock.Anything, mock.Anything)
}

func (suite *SignalHandlerTestSuite) TestSignalHandler_ServeHTTP_PublisherFailure() {
	encoded, _ := json.Marshal(suite.Event)
	req := httptest.NewRequest("POST", suite.URL, bytes.NewReader(encoded))
	w := httptest.NewRecorder()

	suite.Publish.Return(errors.New("yikes"))

	suite.Handler.ServeHTTP(w, req)

	suite.Equal(500, w.Result().StatusCode)
	suite.Lock.AssertExpectations(suite.T())
	suite.Publisher.AssertExpectations(suite.T())
}


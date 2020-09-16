package mocks

import (
	"github.com/stretchr/testify/mock"
	"net/http"
)

type SessionParameterHandler struct {
	mock.Mock
}

func (m *SessionParameterHandler) Handle(callId string, sessionId string) http.Handler {
	args := m.Called(callId, sessionId)
	return args.Get(0).(http.Handler)
}

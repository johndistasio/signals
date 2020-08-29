package mocks

import (
	"context"
	"github.com/stretchr/testify/mock"
	"time"
)

type SessionStore struct {
	mock.Mock
}

func (m *SessionStore) Create(ctx context.Context, d time.Duration) (string, error) {
	args := m.Called(ctx, d)
	return args.String(0), args.Error(1)
}

func (m *SessionStore) Read(ctx context.Context, id string) (time.Duration, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(time.Duration), args.Error(1)
}

func (m *SessionStore) Update(ctx context.Context, id string, d time.Duration) error {
	args := m.Called(ctx, id, d)
	return args.Error(0)
}

func (m *SessionStore) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

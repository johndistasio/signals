package mocks

import (
	"context"
	"github.com/stretchr/testify/mock"
)

type Semaphore struct {
	mock.Mock
}

func (m *Semaphore) Acquire(ctx context.Context, name string, id string) (bool, error) {
	args := m.Called(ctx, name, id)
	return args.Bool(0), args.Error(1)
}

func (m *Semaphore) Check(ctx context.Context, name string, id string) (bool, error) {
	args := m.Called(ctx, name, id)
	return args.Bool(0), args.Error(1)
}

func (m *Semaphore) Release(ctx context.Context, name, id string) error {
	args := m.Called(ctx, name, id)
	return args.Error(0)
}

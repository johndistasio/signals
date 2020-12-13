package mocks

import (
	"context"
	"github.com/stretchr/testify/mock"
)

type Publisher struct {
	mock.Mock
}

func (m *Publisher) Publish(ctx context.Context, channel string, event interface{}) error {
	args := m.Called(ctx, channel, event)
	return args.Error(0)
}

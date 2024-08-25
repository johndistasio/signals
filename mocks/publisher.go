package mocks

import (
	"context"
	"github.com/stretchr/testify/mock"
)

type Publisher struct {
	mock.Mock
}

func (m *Publisher) Publish(ctx context.Context, channel string, payload []byte) error {
	args := m.Called(ctx, channel, payload)
	return args.Error(0)
}

package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/johndistasio/signaling/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io"
	"testing"
)

func TestRedisSession(t *testing.T) {
	s := NewRedisSession(nil, 0, "test")
	assert.Equal(t, "test", s.ID())
}

func TestRedisSessionCreate(t *testing.T) {
	cmd := redis.NewBoolResult(true, nil)
	rdb := new(mocks.Redis)
	rdb.On("SetNX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(cmd)

	s := NewRedisSession(rdb, 0, "test")
	err := s.Create(context.Background())

	assert.Nil(t, err)
}

func TestRedisSessionCreateExisting(t *testing.T) {
	cmd := redis.NewBoolResult(false, nil)
	rdb := new(mocks.Redis)
	rdb.On("SetNX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(cmd)

	s := NewRedisSession(rdb, 0, "test")
	err := s.Create(context.Background())

	assert.Equal(t, ErrSessionExists, err)
}

func TestRedisSessionCreateFailure(t *testing.T) {
	cmd := redis.NewBoolResult(false, io.EOF)
	rdb := new(mocks.Redis)
	rdb.On("SetNX", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(cmd)

	s := NewRedisSession(rdb, 0, "test")
	err := s.Create(context.Background())

	assert.Equal(t, ErrSessionBackend, err)
}

func TestRedisSessionRenew(t *testing.T) {
	cmd := redis.NewBoolResult(true, nil)
	rdb := new(mocks.Redis)
	rdb.On("Expire", mock.Anything, mock.Anything, mock.Anything).Return(cmd)

	s := NewRedisSession(rdb, 0, "test")
	err := s.Renew(context.Background())

	assert.Nil(t, err)
}

func TestRedisSessionRenewMissing(t *testing.T) {
	cmd := redis.NewBoolResult(false, nil)
	rdb := new(mocks.Redis)
	rdb.On("Expire", mock.Anything, mock.Anything, mock.Anything).Return(cmd)

	s := NewRedisSession(rdb, 0, "test")
	err := s.Renew(context.Background())

	assert.Equal(t, ErrSessionUnknown, err)
}

func TestRedisSessionRenewFailure(t *testing.T) {
	cmd := redis.NewBoolResult(false, io.EOF)
	rdb := new(mocks.Redis)
	rdb.On("Expire", mock.Anything, mock.Anything, mock.Anything).Return(cmd)

	s := NewRedisSession(rdb, 0, "test")
	err := s.Renew(context.Background())

	assert.Equal(t, ErrSessionBackend, err)
}

func TestRedisSessionExpire(t *testing.T) {
	cmd := redis.NewIntResult(1, nil)
	rdb := new(mocks.Redis)
	rdb.On("Del", mock.Anything, mock.Anything).Return(cmd)

	s := NewRedisSession(rdb, 0, "test")
	err := s.Expire(context.Background())

	assert.Nil(t, err)
}

func TestRedisSessionExpireMissing(t *testing.T) {
	cmd := redis.NewIntResult(0, nil)
	rdb := new(mocks.Redis)
	rdb.On("Del", mock.Anything, mock.Anything).Return(cmd)

	s := NewRedisSession(rdb, 0, "test")
	err := s.Expire(context.Background())

	assert.Equal(t, ErrSessionUnknown, err)
}

func TestRedisSessionExpireFailure(t *testing.T) {
	cmd := redis.NewIntResult(0, io.EOF)
	rdb := new(mocks.Redis)
	rdb.On("Del", mock.Anything, mock.Anything).Return(cmd)

	s := NewRedisSession(rdb, 0, "test")
	err := s.Expire(context.Background())

	assert.Equal(t, ErrSessionBackend, err)
}

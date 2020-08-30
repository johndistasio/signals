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

func TestRedisSemaphore_Acquire(t *testing.T) {
	result := redis.NewCmdResult(int64(1), nil)

	rdb := new(mocks.Redis)

	rdb.On("Eval", mock.Anything, mock.Anything, []string{"testKey"}, mock.Anything).Return(result)

	sem := &RedisSemaphore{Redis: rdb}

	acq, err := sem.Acquire(context.Background(), "testKey", "testId")

	assert.True(t, acq)
	assert.Nil(t, err)
}

func TestRedisSemaphore_Acquire_Failure(t *testing.T) {
	result := redis.NewCmdResult(int64(0), nil)

	rdb := new(mocks.Redis)

	rdb.On("Eval", mock.Anything, mock.Anything, []string{"testKey"}, mock.Anything).Return(result)

	sem := &RedisSemaphore{Redis: rdb}

	acq, err := sem.Acquire(context.Background(), "testKey", "testId")

	assert.False(t, acq)
	assert.Nil(t, err)
}

func TestRedisSemaphore_Acquire_Error(t *testing.T) {
	result := redis.NewCmdResult(int64(0), io.EOF)

	rdb := new(mocks.Redis)

	rdb.On("Eval", mock.Anything, mock.Anything, []string{"testKey"}, mock.Anything).Return(result)

	sem := &RedisSemaphore{Redis: rdb}

	acq, err := sem.Acquire(context.Background(), "testKey", "testId")

	assert.False(t, acq)
	assert.Equal(t, ErrSemaphoreGone, err)
}

func TestRedisSemaphore_Release(t *testing.T) {
	result := redis.NewIntResult(1, nil)

	rdb := new(mocks.Redis)

	rdb.On("ZRem", mock.Anything, "testKey", []interface{}{"testId"}).Return(result)

	sem := &RedisSemaphore{Redis: rdb}

	err := sem.Release(context.Background(), "testKey", "testId")

	assert.Nil(t, err)
}

func TestRedisSemaphore_Release_Error(t *testing.T) {
	result := redis.NewIntResult(0, io.EOF)

	rdb := new(mocks.Redis)

	rdb.On("ZRem", mock.Anything, "testKey", []interface{}{"testId"}).Return(result)

	sem := &RedisSemaphore{Redis: rdb}

	err := sem.Release(context.Background(), "testKey", "testId")

	assert.Equal(t, ErrSemaphoreGone, err)
}

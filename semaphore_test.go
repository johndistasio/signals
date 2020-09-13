package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/johndistasio/signaling/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"io"
	"testing"
)

const RedisSemaphoreTestKey = "testKey"

const RedisSemaphoreTestId = "testId"

type RedisSemaphoreTestSuite struct {
	suite.Suite
	mockRedisEval *mock.Call
	mockRedisZRem *mock.Call
	sem           *RedisSemaphore
}

func (suite *RedisSemaphoreTestSuite) SetupTest() {
	rdb := new(mocks.Redis)
	suite.sem = &RedisSemaphore{Redis: rdb}

	key := lockKeyPrefix+RedisSemaphoreTestKey

	suite.mockRedisEval = rdb.On("Eval", mock.Anything, mock.Anything, []string{key}, mock.Anything)
	suite.mockRedisZRem = rdb.On("ZRem", mock.Anything, key, []interface{}{RedisSemaphoreTestId})
}

func TestRedisSemaphoreTestSuite(t *testing.T) {
	suite.Run(t, new(RedisSemaphoreTestSuite))
}

func (suite *RedisSemaphoreTestSuite) TestAcquire() {
	suite.mockRedisEval.Return(redis.NewCmdResult(int64(1), nil))

	acq, err := suite.sem.Acquire(context.Background(), RedisSemaphoreTestKey, RedisSemaphoreTestId)

	suite.True(acq)
	suite.Nil(err)
}

func (suite *RedisSemaphoreTestSuite) TestAcquire_Failure() {
	suite.mockRedisEval.Return(redis.NewCmdResult(int64(0), nil))

	acq, err := suite.sem.Acquire(context.Background(), RedisSemaphoreTestKey, RedisSemaphoreTestId)

	suite.False(acq)
	suite.Nil(err)
}

func (suite *RedisSemaphoreTestSuite) TestAcquire_Error() {
	suite.mockRedisEval.Return(redis.NewCmdResult(int64(0), io.EOF))

	acq, err := suite.sem.Acquire(context.Background(), RedisSemaphoreTestKey, RedisSemaphoreTestId)

	suite.False(acq)
	suite.Equal(ErrSemaphoreGone, err)
}

func (suite *RedisSemaphoreTestSuite) TestAcquire_BadInput() {
	suite.mockRedisEval.Return(redis.NewCmdResult(int64(0), nil))

	acq, err := suite.sem.Acquire(context.Background(), "", "")

	suite.False(acq)
	suite.Equal(ErrSemaphore, err)
}

func (suite *RedisSemaphoreTestSuite) TestCheck() {
	suite.mockRedisEval.Return(redis.NewCmdResult(int64(1), nil))

	acq, err := suite.sem.Check(context.Background(), RedisSemaphoreTestKey, RedisSemaphoreTestId)

	suite.True(acq)
	suite.Nil(err)
}

func (suite *RedisSemaphoreTestSuite) TestCheck_Error() {
	suite.mockRedisEval.Return(redis.NewCmdResult(int64(0), io.EOF))

	acq, err := suite.sem.Check(context.Background(), RedisSemaphoreTestKey, RedisSemaphoreTestId)

	suite.False(acq)
	suite.Equal(ErrSemaphoreGone, err)
}

func (suite *RedisSemaphoreTestSuite) TestCheck_BadInput() {
	suite.mockRedisEval.Return(redis.NewCmdResult(int64(0), nil))

	acq, err := suite.sem.Check(context.Background(), "", "")

	suite.False(acq)
	suite.Equal(ErrSemaphore, err)
}

func (suite *RedisSemaphoreTestSuite) TestRelease() {
	suite.mockRedisZRem.Return(redis.NewIntResult(1, nil))

	err := suite.sem.Release(context.Background(), RedisSemaphoreTestKey, RedisSemaphoreTestId)

	suite.Nil(err)
}

func (suite *RedisSemaphoreTestSuite) TestRelease_Error() {
	suite.mockRedisZRem.Return(redis.NewIntResult(0, io.EOF))

	err := suite.sem.Release(context.Background(), RedisSemaphoreTestKey, RedisSemaphoreTestId)

	suite.Equal(ErrSemaphoreGone, err)
}

func (suite *RedisSemaphoreTestSuite) TestRelease_BadInput() {
	suite.mockRedisZRem.Return(redis.NewIntResult(0, io.EOF))

	err := suite.sem.Release(context.Background(), "", "")

	suite.Equal(ErrSemaphore, err)
}

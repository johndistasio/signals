package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/johndistasio/signaling/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"net"
	"testing"
)

const RedisPublisherTestChannel = "test"

type RedisPublisherTestSuite struct {
	suite.Suite
	mockRedisPublish *mock.Call
	pub              *RedisPublisher
}

func (suite *RedisPublisherTestSuite) SetupTest() {
	rdb := new(mocks.Redis)
	suite.mockRedisPublish = rdb.On("Publish", mock.Anything, RedisPublisherTestChannel, mock.Anything)
	suite.pub = &RedisPublisher{rdb}
}

func TestRedisPublisherTestSuite(t *testing.T) {
	suite.Run(t, new(RedisPublisherTestSuite))
}

func (suite *RedisPublisherTestSuite) TestPublish() {
	suite.mockRedisPublish.Return(redis.NewIntResult(int64(0), nil))

	payload := []byte("hello")

	err := suite.pub.Publish(context.Background(), RedisPublisherTestChannel, payload)

	suite.Nil(err)
}

func (suite *RedisPublisherTestSuite) TestPublish_Error() {
	suite.mockRedisPublish.Return(redis.NewIntResult(int64(0), &net.OpError{}))

	payload := []byte("hello")

	err := suite.pub.Publish(context.Background(), "test", payload)

	suite.Equal(ErrPublisherGone, err)
}

func (suite *RedisPublisherTestSuite) TestPublish_BadInput() {
	suite.mockRedisPublish.Return(redis.NewIntResult(int64(0), nil))

	err := suite.pub.Publish(context.Background(), "test", []byte{})

	suite.Equal(ErrPublisher, err)
}

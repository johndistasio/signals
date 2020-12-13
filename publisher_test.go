package main

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/johndistasio/signals/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"net"
	"testing"
)

const RedisPublisherTestChannel = "test"

type RedisPublisherTestSuite struct {
	suite.Suite
	Topic string
	Channel string
	Event Event
	Publisher *RedisPublisher
	Publish   *mock.Call
}

func (suite *RedisPublisherTestSuite) SetupTest() {
	rdb := new(mocks.Redis)

	suite.Topic = "test"

	suite.Event = Event {
		Call: suite.Topic,
		Session: "12345",
	}

	encoded, _ := json.Marshal(suite.Event)

	suite.Publish = rdb.On("Publish", mock.Anything, RedisTopicPrefix + suite.Topic, encoded)
	suite.Publisher = &RedisPublisher{rdb}
}

func TestRedisPublisherTestSuite(t *testing.T) {
	suite.Run(t, new(RedisPublisherTestSuite))
}

func (suite *RedisPublisherTestSuite) TestPublish() {
	suite.Publish.Return(redis.NewIntResult(int64(0), nil))

	err := suite.Publisher.Publish(context.Background(), suite.Topic, suite.Event)

	suite.Nil(err)
}

func (suite *RedisPublisherTestSuite) TestPublish_Error() {
	suite.Publish.Return(redis.NewIntResult(int64(0), &net.OpError{}))

	err := suite.Publisher.Publish(context.Background(), suite.Topic, suite.Event)

	suite.Equal(ErrPublisherGone, err)
}

func (suite *RedisPublisherTestSuite) TestPublish_BadInput() {
	suite.Publish.Return(redis.NewIntResult(int64(0), nil))

	err := suite.Publisher.Publish(context.Background(), "", suite.Event)

	suite.Equal(ErrPublisher, err)
}

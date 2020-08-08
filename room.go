package main

import (
	"context"
	"errors"
)

var ErrRoomFull = errors.New("room full")

var ErrRoomGone = errors.New("room backend gone")

type Room interface {
	// Name returns the identifier for the room.
	Name() string

	// Join or re-join the room. Returns ErrRoomFull if there are no more empty seats or ErrRoomGone if the
	// reservation could not be completed.
	Join(ctx context.Context) error

	// Leave the room. This should always succeed; either we were able to successfully revoke our reservation, or we
	// were unable to connect with reservation backend and our reservation expire. In the case of the latter, we return
	// an ErrRoomGone as a hint to the caller about the state of the backend.
	Leave(ctx context.Context) error

	// Publish a message to other members of the room. No-op if we haven't yet joined a room. Returns an ErrRoomGone
	// if we could not communicate with the messaging backend.
	Publish(ctx context.Context, message []byte) error

	// Receive a message from other members of the room. No-op if we haven't yet joined a room. Returns an ErrRoomGone
	// if we could not communicate with the messaging backend.
	Receive(ctx context.Context) ([]byte, error)
}
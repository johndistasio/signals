package main

import (
	"context"
	"errors"
)

var ErrRoomFull = errors.New("room full")

var ErrRoomGone = errors.New("room backend gone")

var ErrRoomLeft = errors.New("not in room")

type Room interface {
	// Name returns the identifier for the room.
	Name() string

	// Enter the room (or update an existing reservation) using the given id, evicting other members with reservations
	// older than s seconds ago.
	// Returns the reservation time as a Unix timestamp on success.
	// Returns ErrRoomFull if there are no more empty seats or ErrRoomGone if the reservation could not be completed.
	Enter(ctx context.Context, id string, s int64) (int64, error)

	// Leave the room by relinquishing the reservation with the given ID.
	//
	// This should always succeed; either we were able to successfully revoke our reservation, or we were unable to
	// connect with reservation backend and our reservation expire. In the case of the latter, we return
	// an ErrRoomGone as a hint to the caller about the state of the backend.
	Leave(ctx context.Context, id string) error

	// Publish a message to other members of the room. Returns an ErrRoomLeft if we aren't in a room  or an  ErrRoomGone
	// if we could not communicate with the messaging backend.
	Publish(ctx context.Context, message []byte) error

	// Receive a message from other members of the room. Returns an ErrRoomLeft if we aren't in a room or an ErrRoomGone
	// if we could not communicate with the messaging backend.
	Receive(ctx context.Context) ([]byte, error)
}

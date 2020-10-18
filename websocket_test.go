package main

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestValidateClientHandshake(t *testing.T) {
	tests := map[Event]error{
		Event{
			Call:    "test",
			Session: "test",
			Kind:    MessageKindJoin,
		}: nil,
		Event{
			Call:    "test",
			Session: "test",
			Kind:    MessageKindOffer,
		}: ErrBadKind,
		Event{
			Call:    "",
			Session: "test",
			Kind:    MessageKindJoin,
		}: ErrNoCall,
		Event{
			Call:    "test",
			Session: "",
			Kind:    MessageKindJoin,
		}: ErrNoSession,
	}

	for event, expected := range tests {
		err := ValidateClientHandshake(context.Background(), event)
		assert.Equal(t, expected, err)
	}
}

func TestValidatePeerEvent(t *testing.T) {
	call := "testCall"

	tests := map[Event]error{
		Event{
			Call:    call,
			Kind:    MessageKindPeerJoin,
			Session: "testSession",
		}: nil,
		Event{
			Call:    call,
			Kind:    MessageKindJoin,
			Session: "testSession",
		}: ErrBadKind,
		Event{
			Call:    "",
			Kind:    MessageKindPeerJoin,
			Session: "testSession",
		}: ErrBadCall,
		Event{
			Call:    call,
			Kind:    MessageKindPeerJoin,
			Session: "",
		}: ErrNoSession,
	}

	for event, expected := range tests {
		err := ValidatePeerEvent(context.Background(), call, event)
		assert.Equal(t, expected, err)
	}

}

package main

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestValidateClientHandshake(t *testing.T) {
	tests := map[Event]bool{
		Event{
			Call:    "test",
			Session: "test",
			Kind:    MessageKindJoin,
		}: true,
		Event{
			Body:    "test",
			Call:    "test",
			Session: "test",
			Kind:    MessageKindJoin,
		}: false,
		Event{
			Call:    "test",
			Session: "test",
			Kind:    MessageKindOffer,
		}: false,
		Event{
			Call:    "",
			Session: "test",
			Kind:    MessageKindJoin,
		}: false,
		Event{
			Call:    "test",
			Session: "",
			Kind:    MessageKindJoin,
		}: false,
	}

	for event, expected := range tests {
		err := ValidateClientHandshake(context.Background(), event)
		assert.Equal(t, expected, err)
	}
}

func TestValidatePeerEvent(t *testing.T) {
	call := "testCall"

	// TODO split these cases into individual tests
	tests := map[Event]bool{
		Event{
			Call:    call,
			Kind:    MessageKindPeerJoin,
			Session: "testSession",
		}: true,
		Event{
			Body:    "testBody",
			Call:    call,
			Kind:    MessageKindOffer,
			Session: "testSession",
		}: true,
		Event{
			Body:    "testBody",
			Call:    call,
			Kind:    MessageKindAnswer,
			Session: "testSession",
		}: true,
		Event{
			Call:    call,
			Kind:    MessageKindJoin,
			Session: "testSession",
		}: false,
		Event{
			Call:    "",
			Kind:    MessageKindPeerJoin,
			Session: "testSession",
		}: false,
		Event{
			Call:    call,
			Kind:    MessageKindPeerJoin,
			Session: "",
		}: false,
	}

	for event, expected := range tests {
		actual := ValidatePeerEvent(context.Background(), call, event)
		assert.Equal(t, expected, actual)
	}
}

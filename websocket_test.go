package main

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParsePeerEvent(t *testing.T) {
	peer := "testPeer"
	call := "testCall"
	kind := MessageKindPeer

	bytes, _ := json.Marshal(PeerEvent{
		Peer: peer,
		Event: Event{
			Call: call,
			Kind: kind,
		},
	})

	event, err := parsePeerEvent(context.Background(), call, bytes)

	assert.Nil(t, err)

	assert.Equal(t, peer, event.Peer)
	assert.Equal(t, call, event.Call)
	assert.Equal(t, kind, event.Kind)
}

func TestParsePeerEvent_BadCall(t *testing.T) {
	peer := "testPeer"
	call := "testCall"
	kind := MessageKindPeer

	bytes, _ := json.Marshal(PeerEvent{
		Peer: peer,
		Event: Event{
			Call: call,
			Kind: kind,
		},
	})

	_, err := parsePeerEvent(context.Background(), "differentCall", bytes)

	assert.NotNil(t, err)
}

func TestParsePeerEvent_EmptyPeer(t *testing.T) {
	call := "testCall"
	kind := MessageKindPeer

	bytes, _ := json.Marshal(PeerEvent{
		Event: Event{
			Call: call,
			Kind: kind,
		},
	})

	_, err := parsePeerEvent(context.Background(), call, bytes)

	assert.NotNil(t, err)
}

func TestParsePeerEvent_BadMessageKind(t *testing.T) {
	peer := "testPeer"
	call := "testCall"
	kind := MessageKindJoin

	bytes, _ := json.Marshal(PeerEvent{
		Peer: peer,
		Event: Event{
			Call: call,
			Kind: kind,
		},
	})

	_, err := parsePeerEvent(context.Background(), call, bytes)

	assert.NotNil(t, err)
}

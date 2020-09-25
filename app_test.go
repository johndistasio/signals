package main

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEvent_Unmarshal(t *testing.T) {
	raw := `{"kind": "PEER", "body": "test"}`

	var event Event

	err := json.Unmarshal([]byte(raw), &event)

	assert.Nil(t, err)
	assert.Equal(t, MessageKindPeer, event.Kind)
	assert.Equal(t, "test", event.Body)
}

func TestInternalEvent_Marshal(t *testing.T) {
	raw := `{"kind": "PEER", "body": "test", "peer": "testPeer", "call": "testCall"}`

	var event PeerEvent

	err := json.Unmarshal([]byte(raw), &event)

	assert.Nil(t, err)
	assert.Equal(t, MessageKindPeer, event.Kind)
	assert.Equal(t, "test", event.Body)
	assert.Equal(t, "testPeer", event.Peer)
	assert.Equal(t, "testCall", event.Call)
}

package main

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEvent_Unmarshal(t *testing.T) {
	raw := `{"kind": "PEER", "body": "testBody", "call": "testCall", "session": "testSession"}`

	var event Event

	err := json.Unmarshal([]byte(raw), &event)

	assert.Nil(t, err)
	assert.Equal(t, MessageKindPeerJoin, event.Kind)
	assert.Equal(t, "testBody", event.Body)
	assert.Equal(t, "testCall", event.Call)
	assert.Equal(t, "testSession", event.Session)
}

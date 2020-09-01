package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSplitPath_Root(t *testing.T) {
	head, call, op := SplitPath("/call/abc123")
	assert.Equal(t, "call", head)
	assert.Equal(t, "abc123", call)
	assert.Equal(t, "", op)
}

func TestSplitPath_RootSlash(t *testing.T) {
	head, call, op := SplitPath("/call/abc123/")
	assert.Equal(t, "call", head)
	assert.Equal(t, "abc123", call)
	assert.Equal(t, "", op)
}

func TestSplitPath_Operation(t *testing.T) {
	head, call, op := SplitPath("/call/abc123/signal")
	assert.Equal(t, "call", head)
	assert.Equal(t, "abc123", call)
	assert.Equal(t, "signal", op)
}

func TestSplitPath_OperationSlash(t *testing.T) {
	head, call, op := SplitPath("/call/abc123/signal")
	assert.Equal(t, "call", head)
	assert.Equal(t, "abc123", call)
	assert.Equal(t, "signal", op)
}

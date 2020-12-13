package main

import (
	"context"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/segmentio/ksuid"
	"net/http"
	"strings"
)

const MessageKindJoin = "JOIN"

const MessageKindWelcome = "WELCOME"

const MessageKindPeerJoin = "PEER"

const MessageKindOffer = "OFFER"

const MessageKindAnswer = "ANSWER"

type Event struct {
	Call    string `json:"call,omitempty"`
	Session string `json:"session,omitempty"`
	Body    string `json:"body,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

func GenerateSessionId(ctx context.Context) string {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GenerateSessionId")
	defer span.Finish()

	id, err := ksuid.NewRandom()

	if err != nil {
		ext.LogError(span, err)
		return ""
	}

	return id.String()
}

func ParseSessionId(ctx context.Context, id string) bool {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ParseSessionId")
	defer span.Finish()

	_, err := ksuid.Parse(id)

	if err != nil {
		return false
	}

	return true
}

func ExtractCallId(ctx context.Context, r *http.Request) string {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ExtractCallId")
	defer span.Finish()

	segments := strings.Split(strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/"), "/"), "/")

	if len(segments) >= 2 {
		return segments[1]
	}

	return ""
}

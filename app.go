package main

import "errors"

const SessionHeader = "X-Signal-Session"

const MessageKindJoin = "JOIN"

const MessageKindPeerJoin = "PEER"

const MessageKindOffer = "OFFER"

const MessageKindAnswer = "ANSWER"

var ErrBadKind = errors.New("unexpected event kind")

var ErrBadCall = errors.New("unexpected call")

var ErrNoCall = errors.New("no call specified")

var ErrNoSession = errors.New("no session specified")

type Event struct {
	Call    string `json:"call,omitempty"`
	Session string `json:"session,omitempty"`
	Body    string `json:"body,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

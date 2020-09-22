package main

const SessionHeader = "X-Signal-Session"

const MessageKindJoin = "JOIN"

const MessageKindPeer = "PEER"

const MessageKindOffer = "OFFER"

const MessageKindAnswer = "ANSWER"

type Event struct {
	Call    string `json:"call,omitempty"`
	Session string `json:"session,omitempty"`
	Body    string `json:"body,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

type InternalEvent struct {
	Event
	PeerId string `json:"peerId"`
	CallId string `json:"callId"`
}

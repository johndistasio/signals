package main

type Action string

// Outgoing
const ActionCreated = Action("created")
const ActionJoined = Action("joined")

// Incoming
const ActionCreateOrJoin = Action("createorjoin")
const ActionOffer = Action("offer")

type Message struct {
	Action Action `json:"action,omitempty"`
	Room string `json:"room,omitempty"`
	Token string `json:"token,omitempty"`
}


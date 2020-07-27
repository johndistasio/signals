package main

import (
	"errors"
	"sync"
)

type Room struct {
	clients []*TimeTicker
	mu *sync.Mutex
}

func NewRoom() *Room {
	return &Room{
		clients: make([]*TimeTicker, 0),
		mu: &sync.Mutex{},
	}
}

type Rooms struct {
	rooms map[string]*Room
	mu *sync.Mutex
}

func NewRooms() *Rooms {
	return &Rooms{
		rooms: make(map[string]*Room),
		mu: &sync.Mutex{},
	}
}

func (r *Rooms) CreateRoom(room string, client *TimeTicker) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.rooms[room]; exists {
		return errors.New("exists")
	}

	r.rooms[room] = NewRoom()

	return r.joinRoom(room, client)

}

func (r *Rooms) JoinRoom(room string, client *TimeTicker) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.rooms[room]; !exists {
		return errors.New("missing")
	}

	return r.joinRoom(room, client)
}

func (r *Rooms) joinRoom(room string, client *TimeTicker) error {
	r.rooms[room].mu.Lock()
	defer r.rooms[room].mu.Unlock()

	if len(r.rooms[room].clients) > 2 {
		return errors.New("full")
	}

	r.rooms[room].clients = append(r.rooms[room].clients, client)

	return nil
}
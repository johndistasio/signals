package main

import (
	"context"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
)

var upgrader = websocket.Upgrader{}

func ws(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Printf("error on websocket upgrade: %v\n", err)
		w.WriteHeader(500)
		return
	}

	log.Printf("new websocket connection from %s\n", conn.RemoteAddr().String())

	go NewTimeTicker(NewWebsocketProxy(context.Background(), conn)).start()
}

func main() {
	http.HandleFunc("/ws", ws)
	err := http.ListenAndServe(":9000", nil)
	if err != nil {
		log.Fatalf("error: %v\n", err)
	}
}

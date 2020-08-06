package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	_ "net/http/pprof"
)

var upgrader = websocket.Upgrader{}

var rdb = redis.NewClient(&redis.Options{
	Addr:     "localhost:6379",
	Password: "", // no password set
	DB:       0,  // use default DB
})



func ws(w http.ResponseWriter, r *http.Request) {

	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		// The upgrader has already written the error out to the client so we don't need to.
		log.Printf("error on websocket upgrade: %v\n", err)
		return
	}

	log.Printf("new websocket connection from %s\n", r.RemoteAddr)

	// TODO populate a trace with the returned session ID or error
	_, _ = StartSignalRelay(context.Background(), rdb, conn, &SignalRelayOptions{})
}

func main() {
	http.HandleFunc("/ws", ws)
	log.Println("Starting websocket server on :9000")
	err := http.ListenAndServe(":9000", nil)
	if err != nil {
		log.Fatalf("error: %v\n", err)
	}
}

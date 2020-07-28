package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"time"
)

const room = "test"

var upgrader = websocket.Upgrader{}

var rdb = redis.NewClient(&redis.Options{
	Addr:     "localhost:6379",
	Password: "", // no password set
	DB:       0,  // use default DB
})

type SignalRelay struct {
	ctx context.Context
	ws  *WebsocketProxy
	rdb *redis.Client
}

func StartSignalRelay(ctx context.Context, proxy *WebsocketProxy) {
	relay := &SignalRelay{
		ctx: context.Background(),
		ws:  proxy,
		rdb: rdb,
	}

	relay.ws.Start()
	go relay.from()
}

func (s *SignalRelay) from() {
	for {
		select {
		case input := <-s.ws.Recv():
			log.Printf("received %s\n", input)

			err := s.rdb.Publish(s.ctx, "room:test", input).Err()
			if err != nil {
				panic(err)
			}
			/*
				// example of dealing with data received from websocket
				switch string(input) {
				case "goodbye":
					log.Print("hanging up!")
					return
				default:
				}
			*/

		default:
		}
	}
}

func (s *SignalRelay) to() {
}

func ws(w http.ResponseWriter, r *http.Request) {
	remoteAddr := r.RemoteAddr

	log.Printf("new websocket connection from %s\n", remoteAddr)

	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Printf("error on websocket upgrade: %v\n", err)
		w.WriteHeader(500)
		return
	}

	// this shouldn't go here

	others := rdb.HLen(r.Context(), "clients")

	if others.Val() >= 2 {
		log.Printf("room full: %s\n", "clients")
		w.WriteHeader(400)
		return
	}

	pipe := rdb.TxPipeline()

	// Add ourself to the clients table
	// TODO allow for arbitrary rooms
	// TODO use some kind of token as a client ID instead of remote address
	pipe.HSet(r.Context(), "clients", remoteAddr, time.Now().Unix())

	// (Re)set expiration time on the clients table
	pipe.Expire(r.Context(), "clients", 10*time.Second)

	others = pipe.HLen(r.Context(), "clients")
	_, err = pipe.Exec(r.Context())

	if err != nil {
		log.Printf("error on client registration: %v\n", err)
		w.WriteHeader(500)
		return
	}

	log.Printf("there is now %d clients\n", others.Val())

	StartSignalRelay(context.Background(),
		NewWebsocketProxy(context.Background(), conn))
}

func main() {
	http.HandleFunc("/ws", ws)
	log.Println("Starting websocket server on :9000")
	err := http.ListenAndServe(":9000", nil)
	if err != nil {
		log.Fatalf("error: %v\n", err)
	}
}

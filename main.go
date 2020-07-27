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

type SignalRelay struct {
	ctx context.Context
	ws  *WebsocketProxy
	rdb *redis.Client
}

func StartSignalRelay(ctx context.Context, proxy *WebsocketProxy) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	pipe := rdb.TxPipeline()
	incr := pipe.Incr(ctx, room)
	pipe.Expire(ctx, room, time.Minute)

	_, err := pipe.Exec(ctx)
	log.Println(incr.Val(), err)

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
	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Printf("error on websocket upgrade: %v\n", err)
		w.WriteHeader(500)
		return
	}

	log.Printf("new websocket connection from %s\n", conn.RemoteAddr().String())

	StartSignalRelay(r.Context(),
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

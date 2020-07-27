package main

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/segmentio/ksuid"
	"log"
	"net/http"
	"time"
)
// 1. WebsocketClient connects, asks for room
// 2. Service responds with id token and "created" or "joined"

var upgrader = websocket.Upgrader{}

type TimeTicker struct {
	token string
	ws    *WebsocketProxy
}

func NewTimeTicker(proxy *WebsocketProxy) *TimeTicker {
	return &TimeTicker{
		token: ksuid.New().String(),
		ws:    proxy,
	}
}

func (t *TimeTicker) start() {
	t.ws.Start()
	go t.read()
	go t.write()
}

func (t *TimeTicker) read() {
	for {
		select {
		case input := <- t.ws.Recv():
			log.Printf("received %s\n", input)
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

func (t *TimeTicker) write() {
	ticker := time.NewTicker(5 * time.Second)

	t.ws.Send() <- []byte(fmt.Sprintf("the time is %s", time.Now().String()))

	for {
		select {
		case <-ticker.C:
			t.ws.Send() <- []byte(fmt.Sprintf("the time is %s", time.Now().String()))
		default:
		}
	}
}

func ws(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Printf("error: %v\n", err)
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
package main

import (
	"fmt"
	"github.com/segmentio/ksuid"
	"log"
	"time"
)

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
		case input := <-t.ws.Recv():
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

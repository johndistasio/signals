package main

import (
	"context"
	"github.com/gorilla/websocket"
	"log"
	"time"
)

type WebsocketProxy struct {
	cancel context.CancelFunc
	conn *websocket.Conn
	ctx context.Context
	send chan []byte
	recv chan []byte

	// TODO: implement a closed chan
}

func NewWebsocketProxy(ctx context.Context, conn *websocket.Conn) *WebsocketProxy {
	this, cancel := context.WithCancel(ctx)

	return &WebsocketProxy{
		cancel: cancel,
		conn: conn,
		ctx: this,
		send: make(chan []byte),
		recv: make(chan []byte),
	}
}

func (p *WebsocketProxy) Start() {
	// TODO: set up a pong handler so we can react when the remote side goes away
	// TODO: set up read/write size and time limits

	p.conn.SetCloseHandler(func(code int, text string) error {
		p.cancel()

		// Duplicates the behavior from the default close handler.
		// Error ignored since it's always going to be a "closed connection" error.
		message := websocket.FormatCloseMessage(code, "")
		// TODO: use a common write timeout
	    err := p.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(5 * time.Second))
		return err
	})


	go p.doRecv()
	go p.doSend()
}

func (p *WebsocketProxy) Stop() {
	err := p.conn.Close()

	if err != nil {
		log.Printf("connection close error: %v\n", err)
		return
	}
}

func (p *WebsocketProxy) Send()  chan<- []byte {
	return p.send
}

func (p *WebsocketProxy) doSend() {
	ticker := time.NewTicker(30 * time.Second)
	defer p.Stop()
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			log.Printf("doSend stopping")
			return
		case <-ticker.C:
			if err := p.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("pinger error: %v\n", err)
				return
			}
		case message := <-p.send:

			w, err := p.conn.NextWriter(websocket.TextMessage)

			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("writer error on NextWriter: %v\n", err)
				}
				return
			}

			_, err = w.Write(message)

			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("writer error on Write: %v\n", err)
				}
				return
			}

			err = w.Close()

			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("writer error Close: %v\n", err)
				}
				return
			}
		default:
			// intentionally does nothing
		}
	}
}

func (p *WebsocketProxy) Recv() <-chan []byte {
	return p.recv
}

func (p *WebsocketProxy) doRecv() {
	defer p.Stop()

	for {
		select {
		case <-p.ctx.Done():
			log.Printf("doRecv stopping")
			return
		default:
			_, message, err := p.conn.ReadMessage()

			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("reader error on ReadMessage: %v\n", err)
				}

				return
			}

			p.recv <- message
		}
	}
}


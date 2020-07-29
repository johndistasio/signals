package main

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"log"
	"time"
)

// TODO allow for arbitrary rooms
var room = "room:test"
var channel = "channel:test"

type SignalRelay struct {
	cancel context.CancelFunc
	conn *websocket.Conn
	ctx context.Context
	send chan []byte
	recv chan []byte

	// TODO: implement a closed chan
}

type Signal struct {
	PeerId string
	Message string
}

func StartSignalRelay(ctx context.Context, rdb *redis.Client, conn *websocket.Conn) {
	ctx, cancel := context.WithCancel(ctx)

	conn.SetCloseHandler(func(code int, text string) error {
		log.Println("close handler firing")
		StopSignalRelay(ctx, cancel, conn)
		return nil
	})

	conn.SetPongHandler(func(appData string) error {
		log.Println("pong handler firing")

		return nil
	})

	// TODO: set up read/write size and time limits

	go ReadSignal(ctx, cancel, rdb, conn)
	go WriteSignal(ctx, cancel, rdb, conn)
}

func StopSignalRelay(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn) {
	log.Println("StopSignalRelay firing")

	if ctx.Err() == nil {
		cancel()
	}

	_ = conn.Close()

}

func ReadSignal(ctx context.Context, cancel context.CancelFunc, rdb *redis.Client, conn *websocket.Conn) {
	defer StopSignalRelay(ctx, cancel, conn)

	token := conn.RemoteAddr().String()

	for {
		select {
		case <-ctx.Done():
			log.Println("ReadSignal stopping on relay shutdown")
			return
		default:
			// On socket close, this will return an error.
			_, message, err := conn.ReadMessage()

			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("reader error on ReadMessage: %v\n", err)
				}

				log.Println("ReadSignal stopping on socket close")

				return
			}

			// TODO handle error
			msg, _ := json.Marshal(Signal{token, string(message)})

			err = rdb.Publish(ctx, channel, msg).Err()

			if err != nil {
				// TODO add backoff, and figure out how to alert the client of successive failures
				log.Printf("error on publish to redis: %v\n", err)

			}
		}
	}
}

func WriteSignal(ctx context.Context, cancel context.CancelFunc, rdb *redis.Client, conn *websocket.Conn) {
	defer StopSignalRelay(ctx, cancel, conn)

	// TODO shrink this ping time
	ping := time.NewTicker(30 * time.Second)

	token := conn.RemoteAddr().String()

	ch := rdb.Subscribe(ctx, channel)

	defer ch.Close()

	for {
		select {
		case <-ctx.Done():
			log.Printf("%s: WriteSignal stopping on relay shutdown\n", token)
			return
		case <-ping.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("%s: writer error on WriteMessage (pinger): %v\n", token, err)
				}

				return
			}
		case message := <-ch.Channel():
			bMessage := []byte(message.Payload)

			var signal Signal

			// TODO handle error
			_ = json.Unmarshal(bMessage, &signal)

			if signal.PeerId == token {
				continue
			}

			err := conn.WriteMessage(websocket.TextMessage, bMessage)

			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("%s: writer error on WriteMessage: %v\n", token, err)
				} else {
					log.Printf("%s: WriteSignal stopping on socket close\n", token)
				}

				return
			}
		default:
			// intentionally does nothing
		}
	}
}

/*
func register(ctx context.Context, rdb *redis.Client, conn *websocket.Conn) (bool, error) {
	registered := false

	// get members hash for room
	members, err := rdb.HGetAll(ctx, room).Result()

	if err != nil {
		return registered, err
	}

	// TODO use some kind of token as a client ID instead of remote address
	token := conn.RemoteAddr().String()


	// if we're in there, or if we aren't in there and len < 2, add ourselves and update room expiration
	if len(members) < 2 || members[token] != "" {
		pipe := rdb.TxPipeline()

		pipe.HSet(ctx, room, token, time.Now().Unix())

		// (Re)set expiration time on the clients table
		// TODO shrink this timeout
		pipe.Expire(ctx, room, 1*time.Minute)

		_, err = pipe.Exec(ctx)
	}

	// TODO need to kick out members that have been there for too long without a timestamp update

	// else the room is full

	// TODO return an error to the client when the room is full

	return registered, err
}
*/
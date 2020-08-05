package main

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"log"
	"strings"
	"time"
)

// TODO allow for arbitrary rooms
var room = "test"
var channelPrefix = "channel:"
var seatPrefix = "seat:" + room + ":"

// TODO replace with members
var PongTimeout = 60 * time.Second
var WriteTimeout = 10 * time.Second
var PingInterval = 30 * time.Second
var ReadLimit int64 = 512

type SignalRelay struct {
	// Max time between received pongs. If this timeout elapses, we consider the client to be dead.
	PongTimeout time.Duration

	// Max time for message writes to the client. If this timeout elapses, we consider the client to be dead.
	WriteTimeout time.Duration

	// Time between pings sent to the client. Must be less than PongTimeout to give the client a chance to respond.
	PingInterval time.Duration

	id string

	conn *websocket.Conn

	rdb *redis.Client

	seat seat
}

type seat struct {
	room string
	id string
}

func (s *seat) key() string {
	return "seat:" + s.room + ":" + s.id
}

func (s *seat) channelKey() string {
	return "channel:" + s.room
}

type Signal struct {
	PeerId string
	Message string
}

func StartSignalRelay(ctx context.Context, id string, rdb *redis.Client, conn *websocket.Conn) {

	// TODO store id in the db with some kind of long expiration?
	// not sure if we'll need to worry about securing sessions specific to this service if auth is handled elsewhere

	relay := &SignalRelay{
		PongTimeout: PongTimeout,
		WriteTimeout: WriteTimeout,
		PingInterval: PingInterval,
		id: id,
		rdb: rdb,
		conn: conn,
		seat: seat{room, "0"},
	}

	conn.SetCloseHandler(func(code int, text string) error {
		log.Println("close handler firing")
		return nil
	})

	conn.SetPongHandler(func(appData string) error {
		log.Println("pong handler firing")

		// If this errors then the underlying connection is in a bad state, which is unrecoverable.
		err := conn.SetReadDeadline(time.Now().Add(PongTimeout))

		if err == nil {
			// TODO reset peering key expiration
		}

		return err
	})

	conn.SetReadLimit(ReadLimit)

	go relay.ReadSignal()
	go relay.WriteSignal()
}

func (r *SignalRelay) stop() {
	_ = r.conn.Close()
}

func isSocketCloseError(err error) bool {
	if err == websocket.ErrReadLimit {
		// If the client wrote more bytes than is allowed then te socket will be closed.
		return true
	}

	if _, ok := err.(*websocket.CloseError); ok {
		return true
	}

	if strings.Contains(err.Error(), "use of closed network connection") {
		// There doesn't seem to be a better way to detect this error from Go's TCP library. Their own
		// HTTP/2 code does this exact string comparison when checking for a socket close, too.
		return true
	}

	return false
}

func (r *SignalRelay) ReadSignal() {
	defer r.stop()

	for {
		// On socket close, this will return an error.
		_, message, err := r.conn.ReadMessage()

		if err != nil {
			if !isSocketCloseError(err) {
				log.Printf("%s: reader error on ReadMessage: %v\n", r.id, err)
			}
			return
		}

		msg, err := json.Marshal(Signal{r.id, string(message)})

		if err != nil {
			log.Printf("%s: error on serializing message: %v\n", r.id, err)
			return
		}

		err = rdb.Publish(context.TODO(), channelPrefix+room, msg).Err()

		if err != nil {
			log.Printf("%s: error on publish to redis: %v\n", r.id, err)
			return
		}
	}
}

func (r *SignalRelay) WriteSignal() {
	ping := time.NewTicker(PingInterval)

	// TODO subscription needs it's own context and setup function
	ch := rdb.Subscribe(context.TODO(), channelPrefix+room)

	defer func() {
		ping.Stop()
		r.stop()
	}()

	for {
		select {
		case <-ping.C:
			if err := r.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("%s: writer error on WriteMessage (pinger): %v\n", r.id, err)
				}

				return
			}
		case message, ok := <-ch.Channel():
			if !ok {
				log.Printf("%s: redis subscription channel closed\n", r.id)
				return
			}

			bMessage := []byte(message.Payload)

			var signal Signal

			err := json.Unmarshal(bMessage, &signal)

			if err != nil {
				log.Printf("%s: error on deserializing message: %v\n", r.id, err)
				return
			}

			if signal.PeerId == r.id {
				continue
			}

			err = r.conn.WriteMessage(websocket.TextMessage, bMessage)

			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("%s: unexpected error on WriteMessage: %v\n", r.id, err)
				}

				return
			}
		default:
			// intentionally does nothing
		}
	}
}
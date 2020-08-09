package main

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/segmentio/ksuid"
	"log"
	"strings"
	"sync"
	"time"
)

// TODO allow for arbitrary rooms
var room = "test"

var DefaultPongTimeout = 60 * time.Second
var DefaultWriteTimeout = 10 * time.Second
var DefaultPingInterval = 30 * time.Second
var DefaultReadLimit int64 = 512

type SignalRelayOptions struct {
	// Max time for message writes to the client. If this timeout elapses, we consider the client to be dead.
	WriteTimeout time.Duration

	// Max time between received pongs. If this timeout elapses, we consider the client to be dead.
	PongTimeout time.Duration

	// Time between pings sent to the client. Must be less than PongTimeout to give the client a chance to respond.
	PingInterval time.Duration

	// Limit (in bytes) on message reads.
	ReadLimit int64
}

type SignalRelay struct {
	pingInterval time.Duration
	pongTimeout time.Duration
	writeTimeout time.Duration
	readLimit int64

	id string

	conn *websocket.Conn

	rdb Redis


	closeOnce *sync.Once
	closeReader chan struct{}
	closeWriter chan struct{}

	seat seat


	room Room
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

func StartSignalRelay(ctx context.Context, rdb Redis, conn *websocket.Conn, opts *SignalRelayOptions) (sessionId string, err error) {
	id, err :=  ksuid.NewRandom()

	if err != nil {
		return sessionId, err
	}

	sessionId = id.String()

	if opts.PongTimeout < 1 {
		opts.PongTimeout = DefaultPongTimeout
	}

	if opts.WriteTimeout < 1 {
		opts.WriteTimeout = DefaultWriteTimeout
	}

	if opts.PingInterval < 1 {
		opts.PingInterval = DefaultPingInterval
	}

	if opts.ReadLimit < 1 {
		opts.ReadLimit = DefaultReadLimit
	}

	rr := &RedisRoom{name: room, rdb: rdb}

	rr.Join(ctx, sessionId)

	relay := &SignalRelay{
		pongTimeout: opts.PongTimeout,
		writeTimeout: opts.WriteTimeout,
		pingInterval: opts.PingInterval,
		readLimit: opts.ReadLimit,
		id: sessionId,
		rdb: rdb,
		conn: conn,

		room: rr,

		closeOnce: new(sync.Once),
		closeReader: make(chan struct{}),
		closeWriter: make(chan struct{}),

		seat: seat{room, "0"},
	}

	conn.SetCloseHandler(func (code int, text string) error {
		relay.Stop(ctx)
		return nil
	})

	conn.SetPongHandler(func(appData string) error {
		log.Println("pong handler firing")

		// If this errors then the underlying connection is in a bad state, which is unrecoverable.
		err := conn.SetReadDeadline(time.Now().Add(relay.pongTimeout))

		if err != nil {
			return err
		}

		// TODO reset peering key expiration

		return err
	})

	conn.SetReadLimit(relay.readLimit)

	go relay.ReadSignal(ctx)
	go relay.WriteSignal(ctx)

	return sessionId, err
}

func (r *SignalRelay) Stop(ctx context.Context) {
	r.closeOnce.Do(func() {
		r.room.Leave(ctx, r.id)
		close(r.closeReader)
		close(r.closeWriter)
		_ = r.conn.Close()
	})
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

func ch(f func() []byte) <-chan []byte {
	ch := make(chan []byte, 1)

	ch <- f()

	defer close(ch)
	return ch
}

func (r *SignalRelay) ReadSignal(ctx context.Context) {
	defer r.Stop(ctx)

	readChan := make(chan []byte)

	go func() {
		for {
			// This will block until it reads something or the socket closes, in which case it will return an error.
			_, message, err := r.conn.ReadMessage()

			if err != nil {
				if !isSocketCloseError(err) {
					log.Printf("%s: reader error on ReadMessage: %v\n", r.id, err)
				} else {
					log.Printf("%s: expected socket closure on Readmessage: %v\n", r.id, err)
				}

				close(readChan)
				return
			}

			readChan <- message
		}
	}()

	for {
		select {
		case <-r.closeReader:
			return
		case <-ctx.Done():
			return
		case message, ok := <-readChan:
			if !ok {
				log.Printf("%s: reader stopping on readChan: %v\n", r.id, ok)
				return
			}

			msg, err := json.Marshal(Signal{r.id, string(message)})

			if err != nil {
				log.Printf("%s: error on serializing message: %v\n", r.id, err)
				return
			}

			err = r.room.Publish(ctx, msg)

			if err != nil {
				log.Printf("%s: error on publish to room: %v\n", r.id, err)
				return
			}
		default:
			// intentionally does nothing
		}
	}
}

func (r *SignalRelay) WriteSignal(ctx context.Context) {
	ping := time.NewTicker(r.pingInterval)

	defer func() {
		ping.Stop()
		r.Stop(ctx)
	}()

	writeChan := make(chan []byte)


	// FIXME this will leak
	go func() {
		for {
			msg, err := r.room.Receive(ctx)

			if err != nil {
				log.Printf("%s: writer stopping on WriteMessage (room): %v\n", r.id, err)
				close(writeChan)
				return
			}

			writeChan <- msg
		}
	}()

	for {
		select {
		case <-r.closeWriter:
			return
		case <-ctx.Done():
			return
		case <-ping.C:
			if err := r.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("%s: writer error on WriteMessage (pinger): %v\n", r.id, err)
				}

				return
			}
		case message, ok := <-writeChan:
			if !ok {
				log.Printf("%s: writer stopping on writeCHan : %v\n", r.id, ok)
				return
			}

			var signal Signal

			err := json.Unmarshal(message, &signal)

			if err != nil {
				log.Printf("%s: error on deserializing message: %v\n", r.id, err)
				return
			}

			if signal.PeerId == r.id {
				continue
			}

			err = r.conn.WriteMessage(websocket.TextMessage, message)

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
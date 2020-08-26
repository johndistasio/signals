package websocket

import (
	"context"
	"errors"
	impl "github.com/gorilla/websocket"
	"net"
	"net/http"
	"sync"
	"time"
)

// ErrSocketClosed indicates the operation failed because the underlying websocket connection has been closed.
var ErrSocketClosed = errors.New("websocket closed")

// ErrSocketTimeout indicates the operation failed
var ErrSocketTimeout = errors.New("websocket timeout")

var ErrReadLimit = errors.New("websocket read limit exceeded")

type MessageType int

const (
	TextMessage   MessageType = 1
	BinaryMessage MessageType = 2
	CloseMessage  MessageType = 8
	PingMessage   MessageType = 9
	PongMessage   MessageType = 10
)

var DefaultReadLimit int64 = 512
var DefaultReadTimeout = 30 * time.Second
var DefaultWriteTimeout = 30 * time.Second

type Message struct {
	Type    MessageType
	Payload []byte
}

// Websocket declares a high-level interface for interacting with a websocket, useful for when the state of the
// websocket is coupled to application state.
type Websocket interface {
	// Ping sends a ping-type message to the client. Cancellation can be done through the context parameter.
	Ping(context.Context) error

	// Pong send a pong-type message to the client. Cancellation can be done through the context parameter.
	Pong(context.Context) error

	// Send sends a text-type message to the client with the given payload. Cancellation can be done through the
	// context parameter.
	Send(context.Context, []byte) error

	// Receive returns the latest message read from the websocket. All types of messages, control or otherwise, are
	// returned by this function.
	Receive(context.Context) (*Message, error)

	// Close explicitly closes the underlying connection. Any future operations on this Websocket will fail.
	Close()
}

type Options struct {
	// Limit (in bytes) on message reads. If the client sends a message larger than this, we consider the client to
	// compromised in some way (maliciously or otherwise) and the socket will be closed.
	ReadLimit int64

	// Maximum time between reads on the socket.
	ReadTimeout time.Duration

	WriteTimeout time.Duration
}

func (o *Options) Init() {
	if o.ReadLimit <= 0 {
		o.ReadLimit = DefaultReadLimit
	}

	if o.ReadTimeout <= 0 {
		o.ReadTimeout = DefaultReadTimeout
	}

	if o.WriteTimeout <= 0 {
		o.WriteTimeout = DefaultWriteTimeout
	}
}

// Proxy implements the Websocket interface around the gorilla/websocket library.
type Proxy struct {
	conn *impl.Conn
	opts *Options
	recv chan *Message
	err  chan error
	once *sync.Once
}


// Ping implements the Ping function from the Websocket interface.
func (p *Proxy) Ping(ctx context.Context) error {
	return p.write(ctx, PingMessage, nil)
}

// Pong implements the Pong function from the Websocket interface.
func (p *Proxy) Pong(ctx context.Context) error {
	return p.write(ctx, PongMessage, nil)
}

// Send implements the Send function from the Websocket interface.
func (p *Proxy) Send(ctx context.Context, msg []byte) error {
	return p.write(ctx, TextMessage, msg)
}

// Receive implements the Receive function from the Websocket interface.
func (p *Proxy) Receive(ctx context.Context) (*Message, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err, ok := <-p.err:
			if !ok {
				return nil, ErrSocketClosed
			}
			return nil, err
		case msg, ok := <-p.recv:
			if !ok {
				return nil, ErrSocketClosed
			}
			return msg, nil
		default:
		}
	}
}

// Close implements the Close function from the Websocket interface.
func (p *Proxy) Close() {
	p.once.Do(func() {
		close(p.recv)
		close(p.err)
		_ = p.conn.WriteControl(impl.CloseMessage, nil, time.Now().Add(time.Second))
		_ = p.conn.Close()
	})
}

// TODO figure out what options in here we should expose
var u = impl.Upgrader{}

func Upgrade(opts *Options, w http.ResponseWriter, r *http.Request, h http.Header) (Websocket, error) {
	conn, err := u.Upgrade(w, r, h)

	if err != nil {
		return nil, err
	}

	opts.Init()

	p := &Proxy{
		conn: conn,
		opts: opts,
		recv: make(chan *Message),
		err:  make(chan error, 1),
		once: new(sync.Once),
	}

	p.conn.SetReadLimit(p.opts.ReadLimit)

	// These callbacks are the only way to expose control messages to application components. Gorilla executes them on
	// the same goroutine that reads of non-control messages happen on, i.e. the one that the readLoop function runs on.

	p.conn.SetCloseHandler(func(code int, text string) error {
		p.recv <- &Message{Type: CloseMessage, Payload: nil}
		p.Close()
		return nil
	})

	p.conn.SetPingHandler(func(appData string) error {
		_ = p.conn.SetReadDeadline(time.Now().Add(p.opts.ReadTimeout))
		p.recv <- &Message{Type: PingMessage, Payload: []byte(appData)}
		return nil
	})

	p.conn.SetPongHandler(func(appData string) error {
		_ = p.conn.SetReadDeadline(time.Now().Add(p.opts.ReadTimeout))
		p.recv <- &Message{Type: PongMessage, Payload: []byte(appData)}
		return nil
	})

	go p.readLoop()

	return p, err
}

func (p *Proxy) write(ctx context.Context, t MessageType, m []byte) error {
	ch := make(chan error, 1)

	go func() {
		_ = p.conn.SetWriteDeadline(time.Now().Add(p.opts.WriteTimeout))
		err := p.conn.WriteMessage(int(t), m)

		var e net.Error

		if errors.As(err, &e) {
			if e.Timeout() {
				ch <- ErrSocketTimeout
				return
			}

			ch <- ErrSocketClosed
			return
		}

		ch <- err
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-ch:
		return err
	}
}


// readLoop continually reads from the socket. Because of the design of the underlying gorilla/websocket library, this
// must occur for control messages as well as application-level messages are properly received. This function sends
// received application-level messages (i.e. text-type and binary-type messages) as returned from gorilla/websocket's
// ReadMessage function to the channel read by our Receive function. Control messages are added to that channel
// separately, via callbacks.
func (p *Proxy) readLoop() {
	defer p.Close()

	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok && e.Error() == "send on closed channel" {
				return
			}

			panic(r)
		}
	}()

	for {
		_ = p.conn.SetReadDeadline(time.Now().Add(p.opts.ReadTimeout))
		t, data, err := p.conn.ReadMessage()

		if err != nil {
			if errors.Is(err, impl.ErrReadLimit) {
				p.err <- ErrReadLimit
				return
			}

			var e net.Error

			if errors.As(err, &e) && e.Timeout() {
				p.err <- ErrSocketTimeout
				return
			}

			p.err <- ErrSocketClosed
			return
		}

		p.recv <- &Message{Type: MessageType(t), Payload: data}
	}
}

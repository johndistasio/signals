package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/opentracing/opentracing-go"
	"gopkg.in/alecthomas/kingpin.v2"
	"log"
	"net/http"
	_ "net/http/pprof"
)

var (
	addr      = kingpin.Flag("addr", "Host:port for service endpoints.").Envar("SIGNAL_ADDR").Default(":8080").TCP()
	infraAddr = kingpin.Flag("infra-addr", "Host:port for infrastructure endpoints.").Envar("SIGNAL_INFRA_ADDR").Default(":8090").TCP()
	origin    = kingpin.Flag("origin", "Origin for CORS.").Envar("SIGNAL_ORIGIN").Default("http://localhost:8080").URL()

	redisAddr = kingpin.Flag("redis-addr", "Redis host:addr.").Envar("SIGNAL_REDIS_ADDR").Default("localhost:6379").TCP()

	seatCount  = kingpin.Flag("seat-count", "Max clients for signaling session.").Envar("SIGNAL_SEAT_COUNT").Default("2").Int()
	seatMaxAge = kingpin.Flag("seat-max-age", "Max age for signaling session seat.").Envar("SIGNAL_SEAT_MAX_AGE").Default("30s").Duration()

	wsHandshakeTimeout = kingpin.Flag("ws-handshake-timeout", "Max time for websocket upgrade handshake.").Envar("SIGNAL_WS_HANDSHAKE_TIMEOUT").Default("0s").Duration()
	wsJoinTimeout      = kingpin.Flag("ws-join-timeout", "Max time for call join handshake.").Envar("SIGNAL_WS_JOIN_TIMEOUT").Default("0s").Duration()
	wsPingInterval     = kingpin.Flag("ws-ping-interval", "Time between websocket client liveliness check.").Envar("SIGNAL_WS_PING_INTERVAL").Default("5s").Duration()
	wsReadTimeout      = kingpin.Flag("ws-read-timeout", "Max time between websocket reads. Must be greater then ping interval.").Envar("SIGNAL_WS_READ_TIMEOUT").Default("10s").Duration()
)

func main() {
	kingpin.Parse()

	tracer, closer, err := initTracer("signals")

	if err != nil {
		log.Fatalf("fatal: tracer init: %v\n", err)
	}

	// Set the singleton opentracing.Tracer with the Jaeger tracer.
	opentracing.SetGlobalTracer(tracer)
	defer closer.Close()

	rdb := redis.NewClient(&redis.Options{Addr: (*redisAddr).String()})

	http.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, err := rdb.Ping(context.Background()).Result()

		if err != nil {
			log.Printf("error: %v\n", err)
			http.Error(w, "unhealthy", http.StatusInternalServerError)
			return
		}

		if s != "PONG" {
			log.Printf("error: unexpected response from redis: %v\n", s)
			http.Error(w, "unhealthy", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))

	go func() {
		log.Printf("Starting signals infra server on %s\n", (*infraAddr).String())
		err := http.ListenAndServe((*infraAddr).String(), nil)

		if err != nil {
			log.Fatalf("fatal: infra: %v\n", err)
		}
	}()

	locker := &RedisSemaphore{
		Age:   *seatMaxAge,
		Count: *seatCount,
		Redis: rdb,
	}

	publisher := &RedisPublisher{rdb}

	mux := NewTracingMux()

	corsHandler := &CORSHandler{
		Origin: (*origin).String(),
		Headers: []string{
			SessionHeader,
			"Cache-Control",
			"Content-Type",
			"User-Agent",
		},
	}

	sessionHandler := &SessionHandler{
		ExtractCallId:     ExtractCallId,
		CreateSessionId:   GenerateSessionId,
		ValidateSessionId: ParseSessionId,
	}

	callHandler := corsHandler.Handle(
		sessionHandler.Handle(
			&SeatHandler{locker, publisher}))

	signalHandler := corsHandler.Handle(
		sessionHandler.Handle(
			&SignalHandler{locker, publisher, 512}), "POST")

	wsHandler := corsHandler.Handle(
		&WebsocketHandler{
			lock:  locker,
			redis: rdb,
			opts: &WebsocketSessionOptions{
				JoinTimeout:  *wsJoinTimeout,
				ReadTimeout:  *wsReadTimeout,
				PingInterval: *wsPingInterval,
			},
			upgrader: websocket.Upgrader{
				// Delegate XSS prevention to CORSHandler.
				CheckOrigin:      func(*http.Request) bool { return true },
				HandshakeTimeout: *wsHandshakeTimeout,
			},
		})

	mux.Handle("/call/", callHandler)
	mux.Handle("/signal/", signalHandler)
	mux.Handle("/ws", wsHandler)

	server := &http.Server{
		Addr:    (*addr).String(),
		Handler: mux,
	}

	log.Printf("Starting signals server on %s\n", (*addr).String())

	if err = server.ListenAndServe(); err != nil {
		log.Fatalf("fatal: signals: %v\n", err)
	}
}

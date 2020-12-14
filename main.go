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
	addr        = kingpin.Flag("addr", "Host:port for service endpoints.").Envar("SIGNALS_ADDR").Default(":8080").TCP()
	healthzAddr = kingpin.Flag("healthz-addr", "Host:port for healthcheck endpoints.").Envar("SIGNALS_HEALTHZ_ADDR").Default(":8090").TCP()
	origin      = kingpin.Flag("origin", "Origin for CORS.").Envar("SIGNALS_ORIGIN").Default("http://localhost:8080").URL()

	redisAddr = kingpin.Flag("redis-addr", "Redis host:addr.").Envar("SIGNALS_REDIS_ADDR").Default("localhost:6379").TCP()

	seatCount   = kingpin.Flag("seat-count", "Max clients for signaling session.").Envar("SIGNALS_SEAT_COUNT").Default("2").Int()
	seatTimeout = kingpin.Flag("seat-timeout", "Timeout for signaling session seat.").Envar("SIGNALS_SEAT_TIMEOUT").Default("30s").Duration()

	wsHandshakeTimeout = kingpin.Flag("ws-handshake-timeout", "Max time for websocket upgrade handshake (zero = no timeout).").Envar("SIGNALS_WS_HANDSHAKE_TIMEOUT").Default("0s").Duration()
	wsJoinTimeout      = kingpin.Flag("ws-join-timeout", "Max time for call join handshake (zero = no timeout).").Envar("SIGNALS_WS_JOIN_TIMEOUT").Default("0s").Duration()
	wsPingInterval     = kingpin.Flag("ws-ping-interval", "Time between websocket client liveliness check.").Envar("SIGNALS_WS_PING_INTERVAL").Default("5s").Duration()
	wsReadTimeout      = kingpin.Flag("ws-read-timeout", "Max time between websocket reads. Must be greater then ping interval.").Envar("SIGNALS_WS_READ_TIMEOUT").Default("10s").Duration()
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

	// Initialize application server
	mux := NewTracingMux()

	locker := &RedisSemaphore{
		Age:   *seatTimeout,
		Count: *seatCount,
		Redis: rdb,
	}

	publisher := &RedisPublisher{rdb}

	corsOrigin := (*origin).String()
	corsHeaders := []string{"Cache-Control", "Content-Type", "User-Agent"}

	getMiddleware := &CORSMiddleware{Origin: corsOrigin, Headers: corsHeaders, Methods: []string{"GET"}}
	postMiddleware := &CORSMiddleware{Origin: corsOrigin, Headers: corsHeaders, Methods: []string{"POST"}}
	jsonMiddleware := &ContentTypeMiddleware{MimeTypes: []string{"application/json"}}

	callHandler := getMiddleware.Handle(&SeatHandler{GenerateSessionId, locker, publisher})

	signalHandler := postMiddleware.Handle(jsonMiddleware.Handle(&SignalHandler{
		Lock:      locker,
		Publisher: publisher,
		MaxRead:   512,
	}))

	wsHandler := getMiddleware.Handle(
		&WebsocketHandler{
			lock:  locker,
			redis: rdb,
			opts: &WebsocketSessionOptions{
				JoinTimeout:  *wsJoinTimeout,
				ReadTimeout:  *wsReadTimeout,
				PingInterval: *wsPingInterval,
			},
			upgrader: websocket.Upgrader{
				// Delegate XSS prevention to CORSMiddleware.
				CheckOrigin:      func(*http.Request) bool { return true },
				HandshakeTimeout: *wsHandshakeTimeout,
			},
		})

	mux.HandleName("/call/", "/call/{call}", callHandler)
	mux.Handle("/signal", signalHandler)
	mux.Handle("/ws", wsHandler)

	server := &http.Server{
		Addr:    (*addr).String(),
		Handler: mux,
	}

	// Initialize healthchecks using the default server
	http.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	// Start healthcheck server
	go func() {
		addr := (*healthzAddr).String()

		log.Printf("Starting signals healthcheck server on %s\n", addr)

		err := http.ListenAndServe(addr, nil)

		if err != nil {
			log.Fatalf("fatal: healthcheck server: %v\n", err)
		}
	}()

	// Start application server
	log.Printf("Starting signals server on %s\n", (*addr).String())

	if err = server.ListenAndServe(); err != nil {
		log.Fatalf("fatal: signals: %v\n", err)
	}
}

package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jaegerConfig "github.com/uber/jaeger-client-go/config"
	jaegerLog "github.com/uber/jaeger-client-go/log"
	jaegerMetrics "github.com/uber/jaeger-lib/metrics"
	"gopkg.in/alecthomas/kingpin.v2"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
)

var (
	devel     = kingpin.Flag("devel", "Enable development mode.").Envar("SIGNAL_DEVEL").Bool()
	debugPort = kingpin.Flag("debug-port", "Host and port for debug endpoints.").Envar("SIGNAL_DEBUG_ADDR").Default(":8090").TCP()
	port      = kingpin.Flag("port", "Host and port for service endpoints.").Envar("SIGNAL_ADDR").Default(":8080").TCP()

	redisAddr = kingpin.Flag("redis-addr", "Redis host and port.").Envar("SIGNAL_REDIS_ADDR").Default("localhost:6379").TCP()

	seatCount  = kingpin.Flag("seat-count", "Max clients for signaling session.").Envar("SIGNAL_SEAT_COUNT").Default("2").Int()
	seatMaxAge = kingpin.Flag("seat-max-age", "Max age for signaling session seat.").Envar("SIGNAL_SEAT_MAX_AGE").Default("30s").Duration()

	wsPingInterval = kingpin.Flag("ws-ping-interval", "Time between websocket client liveliness check.").Envar("SIGNAL_WS_PING_INTERVAL").Default("5s").Duration()
	wsReadTimeout  = kingpin.Flag("ws-read-timeout", "Max time between websocket reads. Must be greater then ping interval.").Envar("SIGNAL_WS_READ_TIMEOUT").Default("10s").Duration()
)

func initTracer() (opentracing.Tracer, io.Closer, error) {
	// Sample configuration for testing. Use constant sampling to sample every trace
	// and enable LogSpan to log every span via configured Logger.
	cfg := jaegerConfig.Configuration{
		ServiceName: "signaling",
		Sampler: &jaegerConfig.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegerConfig.ReporterConfig{
			LogSpans: false,
		},
	}

	// Example logger and metrics factory. Use github.com/uber/jaeger-client-go/log
	// and github.com/uber/jaeger-lib/metrics respectively to bind to real logging and metrics
	// frameworks.
	jLogger := jaegerLog.StdLogger
	jMetricsFactory := jaegerMetrics.NullFactory

	// Initialize tracer with a logger and a metrics factory
	return cfg.NewTracer(
		jaegerConfig.Logger(jLogger),
		jaegerConfig.Metrics(jMetricsFactory),
	)
}

func main() {
	// TODO config
	tracer, closer, err := initTracer()

	if err != nil {
		log.Fatalf("fatal: %v\n", err)
	}

	kingpin.Parse()

	log.Println((*debugPort).String())

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
	}))

	go func() {
		log.Printf("Starting debug server on %s\n", (*debugPort).String())
		err := http.ListenAndServe((*debugPort).String(), nil)

		if err != nil {
			log.Fatalf("fatal: %v\n", err)
		}
	}()

	locker := &RedisSemaphore{
		Age:   *seatMaxAge,
		Count: *seatCount,
		Redis: rdb,
	}

	publisher := &RedisPublisher{rdb}

	session := &SessionHandler{

		// TODO config
		Domain: "",

		// TODO config
		Path: "",

		Insecure:          *devel,
		Javascript:        false,
		CreateSessionId:   GenerateSessionId,
		ValidateSessionId: ParseSessionId,
	}

	seat := &SeatHandler{locker, publisher}
	signal := &SignalHandler{locker, publisher}

	ws := &WebsocketHandler{
		lock:         locker,
		redis:        rdb,
		readTimeout:  *wsReadTimeout,
		pingInterval: *wsPingInterval,

		// TODO config
		upgrader: websocket.Upgrader{},
	}

	app := &App{
		SessionHandler:   session,
		SeatHandler:      seat,
		SignalHandler:    signal,
		WebsocketHandler: ws,
	}

	mux := http.NewServeMux()
	mux.Handle("/", app)

	server := &http.Server{
		Addr:    (*port).String(),
		Handler: mux,
	}

	log.Printf("Starting signaling server on %s\n", (*port).String())
	err = server.ListenAndServe()

	if err != nil {
		log.Fatalf("fatal: %v\n", err)
	}
}

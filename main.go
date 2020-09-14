package main

import (
	"context"
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
	"time"
)

var (
	devel = kingpin.Flag("devel", "Enable development mode.").Envar("SIGNAL_DEVEL").Bool()
	debugPort = kingpin.Arg("debug-port", "Port for debug endpoints").Envar("SIGNAL_DEBUG_PORT").Default(":8090").TCP()
	port = kingpin.Arg("port", "Port for service endpoints").Envar("SIGNAL_PORT").Default(":8080").TCP()
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

	// TODO config
	rdb := NewRedisClient()

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
		// TODO config
		Age: 10 * time.Minute,

		// TODO config
		Count: 2,
		Redis: rdb,
	}

	publisher := &RedisPublisher{rdb}

	// TODO config
	session := &SessionHandler{
		Insecure:          true,
		Javascript:        false,
		CreateSessionId:   GenerateSessionId,
		ValidateSessionId: ParseSessionId,
	}

	seat := &SeatHandler{locker, publisher}
	signal := &SignalHandler{locker, publisher}

	ws := &WebsocketHandler{
		lock:  locker,
		redis: rdb,

		// TODO config
		upgrader: websocket.Upgrader{},

		// TODO config
		readTimeout: 10 * time.Second,

		// TODO config
		pingInterval: 5 * time.Second,
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

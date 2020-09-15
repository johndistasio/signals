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
	"net/url"
	"strings"
)

var (
	addr      = kingpin.Flag("addr", "Host:port for service endpoints.").Envar("SIGNAL_ADDR").Default(":8080").TCP()
	appName   = kingpin.Flag("app-name", "App name for monitoring and observability.").Envar("SIGNAL_APP_NAME").Default("signals").String()
	devel     = kingpin.Flag("devel", "Enable development mode: set insecure cookies and ignore CORS rules.").Envar("SIGNAL_DEVEL").Bool()
	infraAddr = kingpin.Flag("infra-addr", "Host:port for infrastructure endpoints.").Envar("SIGNAL_INFRA_ADDR").Default(":8090").TCP()

	redisAddr = kingpin.Flag("redis-addr", "Redis host:addr.").Envar("SIGNAL_REDIS_ADDR").Default("localhost:6379").TCP()

	seatCount  = kingpin.Flag("seat-count", "Max clients for signaling session.").Envar("SIGNAL_SEAT_COUNT").Default("2").Int()
	seatMaxAge = kingpin.Flag("seat-max-age", "Max age for signaling session seat.").Envar("SIGNAL_SEAT_MAX_AGE").Default("30s").Duration()

	wsOrigin       = kingpin.Flag("ws-origin", "Time between websocket client liveliness check.").Envar("SIGNAL_WS_ORIGIN").Default("http://localhost:8080").URL()
	wsPingInterval = kingpin.Flag("ws-ping-interval", "Time between websocket client liveliness check.").Envar("SIGNAL_WS_PING_INTERVAL").Default("5s").Duration()
	wsReadTimeout  = kingpin.Flag("ws-read-timeout", "Max time between websocket reads. Must be greater then ping interval.").Envar("SIGNAL_WS_READ_TIMEOUT").Default("10s").Duration()
)

func initTracer() (opentracing.Tracer, io.Closer, error) {
	// Sample configuration for testing. Use constant sampling to sample every trace
	// and enable LogSpan to log every span via configured Logger.
	cfg := jaegerConfig.Configuration{
		ServiceName: *appName,
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
	kingpin.Parse()

	// TODO config
	tracer, closer, err := initTracer()

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
	}))

	go func() {
		log.Printf("Starting %s infra server on %s\n", *appName, (*infraAddr).String())
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

	session := &SessionHandler{
		Insecure:          *devel,
		Javascript:        false,
		CreateSessionId:   GenerateSessionId,
		ValidateSessionId: ParseSessionId,
	}

	seat := &SeatHandler{locker, publisher}
	signal := &SignalHandler{locker, publisher}

	var fun func(r *http.Request) bool

	if *devel {
		fun = func(*http.Request) bool {
			return true
		}
	} else {
		fun = func(r *http.Request) bool {
			origin := r.Header["Origin"]

			if len(origin) == 0 {
				return true
			}

			u, err := url.Parse(origin[0])

			if err != nil {
				return false
			}

			return strings.ToLower(u.Host) == strings.ToLower((*wsOrigin).Host)
		}
	}

	ws := &WebsocketHandler{
		lock:         locker,
		redis:        rdb,
		readTimeout:  *wsReadTimeout,
		pingInterval: *wsPingInterval,

		upgrader: websocket.Upgrader{
			CheckOrigin: fun,
		},
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
		Addr:    (*addr).String(),
		Handler: mux,
	}

	log.Printf("Starting %s server on %s\n", *appName, (*addr).String())
	err = server.ListenAndServe()

	if err != nil {
		log.Fatalf("fatal: service: %v\n", err)
	}
}

package main

import (
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jaegerConfig "github.com/uber/jaeger-client-go/config"
	jaegerLog "github.com/uber/jaeger-client-go/log"
	jaegerMetrics "github.com/uber/jaeger-lib/metrics"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"
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
	tracer, closer, err := initTracer()

	if err != nil {
		log.Fatalf("fatal: %v\n", err)
	}

	// Set the singleton opentracing.Tracer with the Jaeger tracer.
	opentracing.SetGlobalTracer(tracer)
	defer closer.Close()

	locker := &RedisSemaphore{
		Age:   10 * time.Second,
		Count: 2,
		Redis: NewRedisClient(),
	}

	session := &SessionMiddleware{
		Insecure:          true,
		Javascript:        false,
		CreateSessionId:   GenerateSessionId,
		ValidateSessionId: ParseSessionId,
	}

	seat := &SeatHandler{locker}
	signal := &SignalHandler{locker}
	ws := &WebsocketHandler{locker}

	router := &RoutingMiddleware{
		SessionMiddleware: session,
		SeatHandler:       seat,
		SignalHandler:     signal,
		WebsocketHandler:  ws,
	}

	http.Handle("/", TraceHandler(router))

	log.Println("Starting signaling server on :9000")

	err = http.ListenAndServe(":9000", nil)
	if err != nil {
		log.Fatalf("fatal: %v\n", err)
	}
}

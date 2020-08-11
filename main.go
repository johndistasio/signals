package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/uber/jaeger-client-go"
	jaegerConfig "github.com/uber/jaeger-client-go/config"
	jaegerLog "github.com/uber/jaeger-client-go/log"
	jaegerMetrics "github.com/uber/jaeger-lib/metrics"
	"log"
	"net/http"
	_ "net/http/pprof"

)

var upgrader = websocket.Upgrader{}

var rdb = redis.NewClient(&redis.Options{
	Addr:     "localhost:6379",
	Password: "", // no password set
	DB:       0,  // use default DB
})



func ws(w http.ResponseWriter, r *http.Request) {
	tracer := opentracing.GlobalTracer()
	sctx, _ := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header))
	span := tracer.StartSpan("/ws", ext.RPCServerOption(sctx))
	defer span .Finish()

	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		ext.LogError(span, err)
		_ = conn.Close()
		return
	}


	ctx := opentracing.ContextWithSpan(context.Background(), span)
	sessionId, err := StartSignalRelay(ctx, rdb, conn, &SignalRelayOptions{})

	if err != nil {
		ext.LogError(span, err)
		_ = conn.Close()
		return
	}

	log.Printf("new signaling session for %s: %s\n", r.RemoteAddr, sessionId)

	span.SetTag("session.id", sessionId)
}

func main() {
	// Sample configuration for testing. Use constant sampling to sample every trace
	// and enable LogSpan to log every span via configured Logger.
	cfg := jaegerConfig.Configuration{
		ServiceName: "signaling",
		Sampler:     &jaegerConfig.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter:    &jaegerConfig.ReporterConfig{
			LogSpans: false,
		},
	}

	// Example logger and metrics factory. Use github.com/uber/jaeger-client-go/log
	// and github.com/uber/jaeger-lib/metrics respectively to bind to real logging and metrics
	// frameworks.
	jLogger := jaegerLog.StdLogger
	jMetricsFactory := jaegerMetrics.NullFactory

	// Initialize tracer with a logger and a metrics factory
	tracer, closer, err := cfg.NewTracer(
		jaegerConfig.Logger(jLogger),
		jaegerConfig.Metrics(jMetricsFactory),
	)

	if err != nil {
		log.Fatalf("fatal: %v\n", err)
	}

	// Set the singleton opentracing.Tracer with the Jaeger tracer.
	opentracing.SetGlobalTracer(tracer)
	defer closer.Close()


	http.HandleFunc("/ws", ws)
	log.Println("Starting websocket server on :9000")
	err = http.ListenAndServe(":9000", nil)
	if err != nil {
		log.Fatalf("fatal: %v\n", err)
	}
}

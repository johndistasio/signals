package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	impl "github.com/gorilla/websocket"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/uber/jaeger-client-go"
	jaegerConfig "github.com/uber/jaeger-client-go/config"
	jaegerLog "github.com/uber/jaeger-client-go/log"
	jaegerMetrics "github.com/uber/jaeger-lib/metrics"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/johndistasio/signaling/websocket"
)

var upgrader = impl.Upgrader{}

var rdb = redis.NewClient(&redis.Options{
	Addr:     "localhost:6379",
	Password: "", // no password set
	DB:       0,  // use gorilla DB
})

var sessionDuration = 60 * time.Minute

func ws(w http.ResponseWriter, r *http.Request) {
	tracer := opentracing.GlobalTracer()
	sctx, _ := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header))
	span := tracer.StartSpan("/ws", ext.RPCServerOption(sctx))
	defer span.Finish()

	ctx := opentracing.ContextWithSpan(context.Background(), span)

	var session Session

	headerId := r.Header.Get("X-Signaling-Session-Id")

	if headerId == "" {
		id, err := GenerateSessionId(ctx)

		if err != nil {
			ext.HTTPStatusCode.Set(span, 500)
			w.WriteHeader(500)
			return
		}

		session = NewRedisSession(rdb, sessionDuration, id)
		span.SetTag("session.id", session.ID())

		if err := session.Create(ctx); err != nil {
			ext.LogError(span, err)
			ext.HTTPStatusCode.Set(span, 500)
			w.WriteHeader(500)
			return
		}
	} else {
		// If there is a header value and it's in the wrong format, bail.
		if !ValidateSessionId(ctx, headerId) {
			ext.HTTPStatusCode.Set(span, 400)
			w.WriteHeader(400)
			return
		}

		session = NewRedisSession(rdb, sessionDuration, headerId)
		span.SetTag("session.id", session.ID())
		span.SetTag("session.reconnect", true)

		if err := session.Renew(ctx); err != nil {
			// If there is a header value and it's for a session we don't know about, bail.
			if err == ErrSessionUnknown {
				ext.HTTPStatusCode.Set(span, 400)
				w.WriteHeader(400)
				return
			}

			ext.LogError(span, err)
			ext.HTTPStatusCode.Set(span, 500)
			w.WriteHeader(500)
			return
		}
	}

	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		ext.LogError(span, err)
		_ = conn.Close()
		return
	}

	err = StartSignalRelay(ctx, session, rdb, conn, &SignalRelayOptions{})

	if err != nil {
		ext.LogError(span, err)
		_ = conn.Close()
		return
	}

	log.Printf("new signaling session for %s: %s\n", r.RemoteAddr, session.ID())
}

func ws2(w http.ResponseWriter, r *http.Request) {
	o := &websocket.Options{
		ReadLimit: 512,
		ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	ws, err := websocket.Upgrade(o, w, r, nil)

	if err != nil {
		log.Println(err)
		return
	}

	err = ws.Ping(context.Background())

	if err != nil {
		log.Printf("1: %#v\n", err)
	}

	ws.Send(context.Background(), []byte("hi"))

	if err != nil {
		log.Printf("2: %#v\n", err)
	}

	ws.Send(context.Background(), []byte("hello"))


	if err != nil {
		log.Printf("3: %#v\n", err)
	}

	for {
		msg, err := ws.Receive(context.Background())

		if err != nil {
			log.Printf("4: %#v\n", err)
			return
		} else {
			log.Printf("5: %#v\n", msg)
		}

		if msg.Type == websocket.PingMessage {
			ws.Pong(context.Background())
		}
	}

}

func main() {
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
	http.HandleFunc("/ws2", ws2)
	log.Println("Starting websocket server on :9000")
	err = http.ListenAndServe(":9000", nil)
	if err != nil {
		log.Fatalf("fatal: %v\n", err)
	}
}

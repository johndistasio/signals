package main

import (
	"fmt"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jaegerConfig "github.com/uber/jaeger-client-go/config"
	jaegerLog "github.com/uber/jaeger-client-go/log"
	jaegerMetrics "github.com/uber/jaeger-lib/metrics"
	"log"
	"net/http"
	_ "net/http/pprof"
	"sort"
)

type HeaderPrinter struct {}

func (c *HeaderPrinter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := `
<!DOCTYPE html>
<html>
<body>
`
	footer := `
</body>
</html>
`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(header))

	headers := make([]string, len(r.Header))

	for k, _ := range r.Header {
		headers = append(headers, k)
	}

	sort.Strings(headers)

	for _, name := range headers {
		for _, value := range r.Header[name] {
			_, _ = w.Write([]byte(fmt.Sprintf("<p><b>%s:</b> %v</p>", name, value)))
		}
	}

	_, _ = w.Write([]byte(footer))
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


	sh := &SessionHandler{
		Insecure:   true,
		Javascript: false,
		Next:              &HeaderPrinter{},
		CreateSessionId:   GenerateSessionId,
		ValidateSessionId: ParseSessionId,
	}

	http.Handle("/session", sh)

	log.Println("Starting websocket server on :9000")

	err = http.ListenAndServe(":9000", nil)
	if err != nil {
		log.Fatalf("fatal: %v\n", err)
	}
}

package main

import (
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/uber/jaeger-client-go"
	jaegerConfig "github.com/uber/jaeger-client-go/config"
	jaegerLog "github.com/uber/jaeger-client-go/log"
	jaegerMetrics "github.com/uber/jaeger-lib/metrics"
	"io"
	"net/http"
)

func initTracer(name string) (opentracing.Tracer, io.Closer, error) {
	// Sample configuration for testing. Use constant sampling to sample every trace
	// and enable LogSpan to log every span via configured Logger.
	cfg := jaegerConfig.Configuration{
		ServiceName: name,
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

// TracingResponseWriter wraps the http.ResponseWriter type and stores a response code so it can be added to a trace
// after child handlers have processed the request.
type TracingResponseWriter struct {
	http.ResponseWriter
	code int
}

// SetCode is an escape hatch for handlers that need to unwrap the underlying http.ResponseWriter for some reason.
func (w *TracingResponseWriter) SetCode(code int) {
	w.code = code
}

// Write implements http.ResponseWriter.
func (w *TracingResponseWriter) Write(b []byte) (int, error) {
	return w.ResponseWriter.Write(b)
}

// WriterHeader implements http.ResponseWriter and stores the HTTP response code.
func (w *TracingResponseWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// Header implements http.ResponseWriter.
func (w *TracingResponseWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

type TracingMux struct {
	*http.ServeMux
}

func NewTracingMux() *TracingMux {
	return &TracingMux{http.NewServeMux()}
}

func (mux *TracingMux) Handle(pattern string, handler http.Handler) {
	wrapper := http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
		tracer := opentracing.GlobalTracer()
		spanCtx, _ := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header))
		span := tracer.StartSpan(pattern, ext.RPCServerOption(spanCtx))
		ctx := opentracing.ContextWithSpan(r.Context(), span)
		defer span.Finish()

		ext.HTTPUrl.Set(span, r.URL.String())
		ext.HTTPMethod.Set(span, r.Method)
		ext.PeerAddress.Set(span, r.RemoteAddr)
		span.SetTag("http.content_type", r.Header["Content-Type"])

		w2 := &TracingResponseWriter{w, 0}

		handler.ServeHTTP(w2, r.WithContext(ctx))

		ext.HTTPStatusCode.Set(span, uint16(w2.code))
	})

	mux.ServeMux.Handle(pattern, wrapper)
}
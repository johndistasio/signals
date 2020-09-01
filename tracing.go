package main

import (
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"net/http"
	"regexp"
)

// TracingResponseWriter wraps the http.ResponseWriter type and stores a response code so it can be added to a trace
// after child handlers have processed the request.
type TracingResponseWriter struct {
	http.ResponseWriter
	code int
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

func TraceHandler(regex *regexp.Regexp, rewrite string, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracer := opentracing.GlobalTracer()
		spanCtx, _ := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header))

		var op string

		if !regex.MatchString(r.URL.Path) {
			op = "TraceHandler"
		} else {
			op = regex.ReplaceAllString(r.URL.Path, rewrite)
		}

		span := tracer.StartSpan(op, ext.RPCServerOption(spanCtx))
		ctx := opentracing.ContextWithSpan(r.Context(), span)
		defer span.Finish()

		ext.HTTPUrl.Set(span, r.URL.String())
		ext.HTTPMethod.Set(span, r.Method)
		ext.PeerAddress.Set(span, r.RemoteAddr)

		trw := &TracingResponseWriter{w, 200}

		handler.ServeHTTP(trw, r.WithContext(ctx))

		ext.HTTPStatusCode.Set(span, uint16(trw.code))
	})
}

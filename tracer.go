package main

import (
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"net/http"
)

var TraceHandler = func(next http.Handler, spanName string) http.Handler {
	return http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
		tracer := opentracing.GlobalTracer()
		spanCtx, _ := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header))
		span := tracer.StartSpan(spanName, ext.RPCServerOption(spanCtx))
		ctx := opentracing.ContextWithSpan(r.Context(), span)
		defer span.Finish()

		ext.HTTPUrl.Set(span, r.URL.String())
		ext.HTTPMethod.Set(span, r.Method)
		ext.PeerAddress.Set(span, r.RemoteAddr)
		span.SetTag("http.content_type", r.Header["Content-Type"])

		w2 := &AppResponseWriter{w, 0}

		next.ServeHTTP(w2, r.WithContext(ctx))

		ext.HTTPStatusCode.Set(span, uint16(w2.code))
	})
}
package main

import (
	"github.com/opentracing/opentracing-go"
	"net/http"
)

type ContentTypeMiddleware struct {
	MimeTypes []string
}

func (c *ContentTypeMiddleware) Handle(next http.Handler) http.Handler {
	mimeMap := make(map[string]int)

	for _, val := range c.MimeTypes {
		mimeMap[val] = 1
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "ContentTypeMiddleware")
		defer span.Finish()

		mime := r.Header["Content-Type"]

		if mime == nil || len(mime) != 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if mimeMap[mime[0]] == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if next != nil  {
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	})
}


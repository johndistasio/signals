package main

import (
	"github.com/opentracing/opentracing-go"
	"net/http"
)

var ContentTypeHandler = func(next http.Handler, mimeTypes ...string) http.Handler {

	mimeMap := make(map[string]int)

	for _, val := range mimeTypes {
		mimeMap[val] = 1
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "ContentTypeHandler")
		defer span.Finish()

		if len(mimeTypes) > 0 {
			mime := r.Header["Content-Type"]

			if mime == nil || len(mime) != 1 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			if mimeMap[mime[0]] == 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}


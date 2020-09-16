package main

import (
	"github.com/opentracing/opentracing-go"
	"net/http"
	"strings"
)

type CORSHandler struct {
	Origin  string
	Headers []string
}

func (c *CORSHandler) Handle(next http.Handler, methods ...string) http.Handler {
	if len(methods) < 1 {
		methods = []string{"GET"}
	}

	methodsHash := make(map[string]int)

	for _, val := range methods {
		methodsHash[strings.ToUpper(val)] = 1
	}

	methodsString := strings.Join(methods, ", ") + ", OPTIONS"

	headersString := strings.Join(c.Headers, ", ")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "CORSHandler")
		defer span.Finish()

		w.Header().Set("Access-Control-Allow-Headers", headersString)
		w.Header().Set("Access-Control-Allow-Methods", methodsString)
		w.Header().Set("Access-Control-Allow-Origin", c.Origin)
		w.Header().Set("Access-Control-Expose-Headers", headersString)
		w.Header().Set("Access-Control-Max-Age", "60")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if methodsHash[r.Method] != 1 {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

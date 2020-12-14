package main

import (
	"github.com/opentracing/opentracing-go"
	"net/http"
	"sort"
	"strings"
)

type CORSMiddleware struct {
	Origin  string
	Headers []string
	Methods []string
}

func (c *CORSMiddleware) Handle(next http.Handler) http.Handler {

	methodsHash := make(map[string]int)

	methodsHash["OPTIONS"] = 1

	for _, val := range c.Methods {
		method := strings.ToUpper(val)
		methodsHash[method] = 1
	}

	pos := 0
	methodsSlice := make([]string, len(methodsHash))
	for method, _ := range methodsHash {
		methodsSlice[pos] = method
		pos++
	}

	sort.Strings(methodsSlice)

	methodsString := strings.Join(methodsSlice, ", ")

	headersString := strings.Join(c.Headers, ", ")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "CORSMiddleware")
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

package main

import (
	"github.com/opentracing/opentracing-go"
	"net/http"
	"sort"
	"strings"
)

var CORSHandler = func(next http.Handler, origin string, methods ...string) http.Handler {
	var options bool

	for i, val := range methods {
		methods[i] = strings.ToUpper(val)

		if methods[i] == "OPTIONS" {
			options = true
		}
	}

	if !options {
		methods = append(methods, "OPTIONS")
	}

	sort.Strings(methods)

	m := strings.Join(methods, ", ")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "CORSHandler")
		defer span.Finish()

		w.Header().Set("Access-Control-Allow-Headers", SeatHeader)
		w.Header().Set("Access-Control-Allow-Methods", m)
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Expose-Headers", SeatHeader)
		w.Header().Set("Access-Control-Max-Age", "60")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		var allowed bool

		for _, method := range methods {
			if method == r.Method {
				allowed = true
			}
		}

		if !allowed {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

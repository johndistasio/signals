package main

import (
	"fmt"
	"net/http"
	"sort"
)

type HeaderPrinter struct{}

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

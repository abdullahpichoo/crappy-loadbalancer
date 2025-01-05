package middleware

import (
	"log"
	"net/http"
)

func LoggingMiddlware(next http.Handler, log *log.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		log.Printf("method %s path: %s", r.Method, r.URL.Path)

		next.ServeHTTP(rw, r)
	}
}

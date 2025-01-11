package server

import (
	"log"
	"net/http"
	"time"

	"github.com/abdullahpichoo/crappy-loadbalancer/backend/middleware"
	"github.com/abdullahpichoo/crappy-loadbalancer/backend/routes"
)

func NewServerHandler(logger *log.Logger) http.Handler {
	mux := http.NewServeMux()
	routes.AddRoutes(mux, logger)

	var handler http.Handler = mux
	handler = middleware.LoggingMiddlware(handler, logger)

	return mux
}

func StartServer(addr string, logger *log.Logger) error {
	handler := NewServerHandler(logger)
	server := http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("server has started on addr %s", addr)
	if err := server.ListenAndServe(); err != nil {
		return err
	}
	return nil
}

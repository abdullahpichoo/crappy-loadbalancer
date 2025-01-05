package main

import (
	"context"
	logger "log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abdullahpichoo/crappy-loadbalancer/loadbalancer/config"
	healthcheck "github.com/abdullahpichoo/crappy-loadbalancer/loadbalancer/health-check"
	"github.com/abdullahpichoo/crappy-loadbalancer/loadbalancer/serverpool"
)

func main() {
	log := logger.New(os.Stdout, "LB:", logger.LstdFlags)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	lbConfig := config.LbConfig{
		MaxNumOfServers:   4,
		DefaultServerAddr: "http://localhost:3000",
		InitialServerAddrs: []string{
			"http://localhost:3000",
		},
		MaxConnsPerServer: 800,
		ServerAddrs: []string{
			"http://localhost:3000", "http://localhost:3001", "http://localhost:3002",
			"http://localhost:3003",
		},
		Strategy: "round-robin",
	}

	serverPool := serverpool.New(lbConfig, log)
	serverPoolHealthChecker := healthcheck.NewHealthChecker(serverPool, 5*time.Second, 8*time.Second)

	if err := serverPool.InitServers(); err != nil {
		log.Fatalln("error booting up servers", err)
	}

	serverPoolHealthChecker.Start(ctx)
	serverPool.LogServerPoolStatus(ctx)

	server := http.Server{
		Addr:         "localhost:8080",
		Handler:      http.HandlerFunc(serverPool.RequestHandler),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		<-ctx.Done()
		log.Println("Shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error during server shutdown: %v\n", err)
		}
	}()

	log.Println("Reverse Proxy is running on localhost:8080")
	if err := server.ListenAndServe(); err != nil {
		log.Printf("Could not start server: %s\n", err.Error())
	}

}

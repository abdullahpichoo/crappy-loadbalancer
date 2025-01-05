package main

import (
	"context"
	logger "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abdullahpichoo/crappy-loadbalancer/loadbalancer/config"
	"github.com/abdullahpichoo/crappy-loadbalancer/loadbalancer/serverpool"
)

func main() {
	log := logger.New(os.Stdout, "LB:", logger.LstdFlags)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	lbConfig := config.LbConfig{
		MaxNumOfServers:   5,
		DefaultServerAddr: "http://localhost:3000",
		InitialServerAddrs: []string{
			"http://localhost:3000", "http://localhost:3001", "http://localhost:3002",
		},
		MaxConnsPerServer: 200,
		ServerAddrs: []string{
			"http://localhost:3000", "http://localhost:3001", "http://localhost:3002",
			"http://localhost:3003", "http://localhost:3004",
		},
	}

	serverPool := serverpool.New(lbConfig, log)
	// serverPoolHealthChecker := healthcheck.NewHealthChecker(serverPool, 5*time.Second, 8*time.Second)

	if err := serverPool.InitServers(); err != nil {
		log.Fatalln("error booting up servers", err)
	}

	// serverPoolHealthChecker.Start()
	serverPool.LogServerPoolStatus(ctx)

	server := http.Server{
		Addr:         "localhost:8080",
		Handler:      http.HandlerFunc(serverPool.RequestHandler),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
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

func handleReq(w http.ResponseWriter, r *http.Request, sPool serverpool.ServerPool) {
	lcServerUrl := sPool.GetLeastConnectionServer()
	reqChan := make(chan string, 1)
	go sPool.RecordRequestLifeCycle(lcServerUrl, reqChan)

	target, err := url.Parse(lcServerUrl)
	if err != nil {
		reqChan <- "failed"
		http.Error(w, "Invalid server URL", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		reqChan <- "failed"
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
	proxy.Director = func(proxyReq *http.Request) {
		proxyReq.URL.Host = target.Host
		proxyReq.URL.Path = r.URL.Path
		proxyReq.Header = r.Header
		proxyReq.URL.Scheme = target.Scheme
	}
	proxy.ModifyResponse = func(r *http.Response) error {
		if r.StatusCode >= 400 && r.StatusCode <= 505 {
			reqChan <- "failed"
		} else {
			reqChan <- "success"
		}
		return nil
	}
	proxy.ServeHTTP(w, r)
}

// create a channel in which health checker sends a signal whether a server is healthy or not
// in that channel I can also show number of healthy servers
// in that channel debug whether a server is being deleted or not

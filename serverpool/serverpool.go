package serverpool

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/abdullahpichoo/crappy-loadbalancer/loadbalancer/config"
	"github.com/abdullahpichoo/crappy-loadbalancer/loadbalancer/server"
)

type serverPool struct {
	servers  []server.ServerInstance
	mux      sync.RWMutex
	lbConfig config.LbConfig
	logger   *log.Logger
	logChan  chan string
}

type ServerPool interface {
	InitServers() error
	GetLeastConnectionServer() string
	AddServer(url string) (server.ServerInstance, error)
	RecordRequestLifeCycle(url string, requestChannel <-chan string) error
	GetAllServers() []server.ServerInstance
	GetActiveServers() []server.ServerInstance
	RemoveServer(url string) error
	RestartServer(url string)
	RequestHandler(w http.ResponseWriter, r *http.Request)
	LogServerPoolStatus(ctx context.Context)
}

func (s *serverPool) InitServers() error {
	for _, url := range s.lbConfig.InitialServerAddrs {
		if srv, err := s.AddServer(url); err != nil {
			srv.Kill()
			return fmt.Errorf("error booting up %s error: %s", url, err)
		}
	}

	return nil
}

func (s *serverPool) GetActiveServers() []server.ServerInstance {
	var servers []server.ServerInstance
	for _, srv := range s.servers {
		if srv.IsAlive() {
			servers = append(servers, srv)
		}
	}
	return servers
}

func (s *serverPool) GetLeastConnectionServer() string {
	servers := s.GetActiveServers()
	if len(servers) == 0 {
		return s.lbConfig.DefaultServerAddr
	}

	var (
		leastConnectionServerUrl = s.lbConfig.DefaultServerAddr
		minConnections           = int32(math.MaxInt32)
		needsNewServer           = true
	)

	for _, srv := range servers {
		stats := srv.GetMetrics()

		if stats.ActiveRequests < int32(s.lbConfig.MaxConnsPerServer) {
			needsNewServer = false
		}

		if stats.ActiveRequests < minConnections {
			minConnections = stats.ActiveRequests
			leastConnectionServerUrl = srv.GetUrl()
		}
	}

	if needsNewServer {
		url, err := bootUpAdditionalServer(s)
		if err != nil {
			return leastConnectionServerUrl
		}
		return url
	}

	return leastConnectionServerUrl
}

func (s *serverPool) RecordRequestLifeCycle(url string, requestChannel <-chan string) error {
	srv, err := findServerByUrl(s, url)

	if err != nil {
		return fmt.Errorf("failed to find the server by url")
	}
	srv.IncrementActiveRequests()
	defer srv.DecrementActiveRequests()

	select {
	case result := <-requestChannel:
		switch result {
		case "success":
			srv.RecordRequestResult(true)
		case "failed":
			srv.RecordRequestResult(false)
		}
	case <-time.After(31 * time.Second):
		srv.RecordRequestResult(false)
	}

	return nil
}

func (s *serverPool) AddServer(url string) (server.ServerInstance, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	server, err := server.New(url)
	if err != nil {
		return nil, err
	}

	_, err = server.BootUp()
	if err != nil {
		return nil, fmt.Errorf("failed to add new server: %s", err.Error())
	}

	s.servers = append(s.servers, server)
	return server, nil
}

func (s *serverPool) GetAllServers() []server.ServerInstance {
	s.mux.RLock()
	defer s.mux.RUnlock()

	return s.servers
}

func (s *serverPool) RestartServer(url string) {
	s.mux.Lock()
	defer s.mux.Unlock()

	server, err := findServerByUrl(s, url)
	if err != nil {
		return
	}

	server.SetIsHealthy(false)
	time.Sleep(200 * time.Millisecond)

	if err := server.Kill(); err != nil {
		return
	}

	_, err = server.BootUp()
	if err != nil {
		return
	}

	server.SetIsHealthy(true)
}

func (s *serverPool) LogServerPoolStatus(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		var msg string
		var msgDuration time.Time

		for {
			select {
			case <-ctx.Done():
				s.logger.Println("gracefully shutting down serverpool")
				s.shutdownServerPool()
				return
			case newMsg := <-s.logChan:
				msg = newMsg
				msgDuration = time.Now().Add(3 * time.Second)
			case <-ticker.C:
				s.logger.Print("\033[H\033[2J")
				s.logger.Println("Server Status:")

				if time.Now().Before(msgDuration) {
					s.logger.Println("----- ", msg)
				} else {
					msg = ""
				}

				for _, server := range s.GetActiveServers() {
					s.logger.Printf("%s Healthy %v - Active: %d Failed: %d Successful: %d\n", server.GetUrl(),
						server.IsHealthy(),
						server.GetMetrics().ActiveRequests,
						server.GetMetrics().FailedRequests,
						server.GetMetrics().SuccessfulRequests)
				}
			}
		}
	}()
}

func (s *serverPool) getValidPeer() (server.ServerInstance, error) {
	servers := s.GetActiveServers()
	if len(servers) == 0 {
		return nil, fmt.Errorf("no server initialized")
	}

	var (
		leastConnectionServer = s.servers[0]
		minConnections        = int32(math.MaxInt32)
		// needsNewServer        = true
	)

	for _, srv := range servers {
		stats := srv.GetMetrics()

		// if stats.ActiveRequests < int32(s.lbConfig.MaxConnsPerServer) {
		// 	needsNewServer = false
		// }

		if stats.ActiveRequests < minConnections {
			minConnections = stats.ActiveRequests
			leastConnectionServer = srv
		}
	}

	// if needsNewServer {
	// 	url, err := bootUpAdditionalServer(s)
	// 	if err != nil {
	// 		return leastConnectionServer, nil
	// 	}
	// 	return url
	// }

	return leastConnectionServer, nil
}

func (s *serverPool) shutdownServerPool() {
	for _, srv := range s.servers {
		s.logger.Println("killing server ", srv.GetUrl())
		if err := srv.Kill(); err != nil {
			s.logger.Println("failed to kill this server")
		}
	}
}

func (s *serverPool) RequestHandler(w http.ResponseWriter, r *http.Request) {
	lcServer, err := s.getValidPeer()
	if err != nil {
		http.Error(w, "failed to find a valid peer", http.StatusInternalServerError)
	}

	lcServer.GetReverseProxy().ServeHTTP(w, r)
}

func New(lbConfig config.LbConfig, logger *log.Logger) ServerPool {
	return &serverPool{
		servers:  []server.ServerInstance{},
		mux:      sync.RWMutex{},
		lbConfig: lbConfig,
		logger:   logger,
		logChan:  make(chan string),
	}
}

func getAddrForNewServer(s *serverPool) (string, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	for _, addr := range s.lbConfig.ServerAddrs {
		if !isServerAlreadyInServerPool(s, addr) {
			return addr, nil
		}
	}

	return "", fmt.Errorf("failed to find new server to add")
}

func isServerAlreadyInServerPool(s *serverPool, addr string) bool {
	for _, srv := range s.GetActiveServers() {
		if addr == srv.GetUrl() {
			return true
		}
	}
	return false
}

func findServerByUrl(s *serverPool, url string) (server.ServerInstance, error) {
	for _, srv := range s.GetActiveServers() {
		if srv.GetUrl() == url {
			return srv, nil
		}
	}
	return nil, fmt.Errorf("failed to find the server with url %s", url)
}

func bootUpAdditionalServer(s *serverPool) (string, error) {
	if len(s.GetActiveServers()) >= int(s.lbConfig.MaxNumOfServers) {
		return "", fmt.Errorf("already max servers reached")
	}
	url, err := getAddrForNewServer(s)
	if err != nil {
		return "", fmt.Errorf("failed to add new server: %v", err)
	}

	_, err = s.AddServer(url)
	if err != nil {
		return "", fmt.Errorf("failed to add new server: %v", err)
	}

	return url, nil
}

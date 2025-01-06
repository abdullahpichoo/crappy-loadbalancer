package serverpool

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abdullahpichoo/crappy-loadbalancer/loadbalancer/config"
	"github.com/abdullahpichoo/crappy-loadbalancer/loadbalancer/server"
)

type serverPool struct {
	servers             []server.ServerInstance
	mux                 sync.RWMutex
	lbConfig            config.LbConfig
	logger              *log.Logger
	logChan             chan string
	currRRServerIdx     int
	additionalServerMux sync.RWMutex
}

type ServerPool interface {
	InitServers() error
	AddServer(url string) (server.ServerInstance, error)
	GetAllServers() []server.ServerInstance
	GetActiveServers() []server.ServerInstance
	RestartServer(url string)
	RequestHandler(w http.ResponseWriter, r *http.Request)
	LogServerPoolStatus(ctx context.Context)
	SendMessage(string)
}

func New(lbConfig config.LbConfig, logger *log.Logger) ServerPool {
	return &serverPool{
		servers:             []server.ServerInstance{},
		mux:                 sync.RWMutex{},
		lbConfig:            lbConfig,
		logger:              logger,
		logChan:             make(chan string),
		additionalServerMux: sync.RWMutex{},
	}
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
		if srv.IsAlive() && srv.IsHealthy() {
			servers = append(servers, srv)
		}
	}
	return servers
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

	isReady := waitTillServerReady(url)
	if !isReady {
		server.Kill()
		s.SendMessage("unable to start new server " + url)
		return nil, fmt.Errorf("unable to start new server %s", url)
	}

	s.servers = append(s.servers, server)
	return server, nil
}

func (s *serverPool) RestartServer(url string) {
	s.mux.Lock()
	defer s.mux.Unlock()

	server, err := findServerByUrl(s, url)
	if err != nil {
		s.SendMessage(err.Error())
		return
	}

	server.SetIsHealthy(false)
	if err := server.Kill(); err != nil {
		s.SendMessage(err.Error())
		return
	}

	time.Sleep(1 * time.Second)

	_, err = server.BootUp()
	if err != nil {
		s.SendMessage(err.Error())
		return
	}

	isReady := waitTillServerReady(url)
	if !isReady {
		server.Kill()
		s.SendMessage("unable to start new server " + url)
		return
	}

	server.SetIsHealthy(true)
}

func (s *serverPool) GetAllServers() []server.ServerInstance {
	s.mux.RLock()
	defer s.mux.RUnlock()

	return s.servers
}

func (s *serverPool) rotate() server.ServerInstance {
	s.mux.Lock()
	s.currRRServerIdx = (s.currRRServerIdx + 1) % len(s.GetActiveServers())
	s.mux.Unlock()
	return s.servers[s.currRRServerIdx]
}

func (s *serverPool) roundRobinNextPeer() (server.ServerInstance, error) {
	for range s.GetActiveServers() {
		nextPeer := s.rotate()
		if !nextPeer.IsAlive() {
			continue
		}
		if nextPeer.GetMetrics().ActiveRequests >= int32(s.lbConfig.MaxConnsPerServer) {
			go func() {
				_, err := bootUpAdditionalServer(s)
				if err != nil {
					s.SendMessage(err.Error())
				}
			}()
		}
		return nextPeer, nil
	}
	return nil, fmt.Errorf("error selecting a valid peer")
}

func (s *serverPool) leastConnectionNextPeer() (server.ServerInstance, error) {
	servers := s.GetActiveServers()
	if len(servers) == 0 {
		return nil, fmt.Errorf("no server initialized")
	}

	var (
		leastConnectionServer = s.servers[0]
		minConnections        = int32(math.MaxInt32)
		needsNewServer        = true
	)

	for _, srv := range servers {
		stats := srv.GetMetrics()

		if stats.ActiveRequests < int32(s.lbConfig.MaxConnsPerServer) {
			needsNewServer = false
		}

		if stats.ActiveRequests < minConnections {
			minConnections = stats.ActiveRequests
			leastConnectionServer = srv
		}
	}

	if needsNewServer {
		go func() {
			_, err := bootUpAdditionalServer(s)
			if err != nil {
				s.SendMessage(err.Error())
			}
		}()

	}

	return leastConnectionServer, nil
}

func (s *serverPool) getValidPeer() (server.ServerInstance, error) {
	strategy := s.lbConfig.Strategy
	if strategy == "round-robin" {
		return s.roundRobinNextPeer()
	} else {
		return s.leastConnectionNextPeer()
	}
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
		s.SendMessage(err.Error())
		http.Error(w, "failed to find a valid peer", http.StatusInternalServerError)
	}

	lcServer.GetReverseProxy().ServeHTTP(w, r)
}

func (s *serverPool) SendMessage(msg string) {
	s.logChan <- msg
}

func getAddrForNewServer(s *serverPool) (string, error) {
	for _, addr := range s.lbConfig.ServerAddrs {
		if !isServerAlreadyInServerPool(s, addr) {
			return addr, nil
		}
	}

	return "", fmt.Errorf("failed to find new server to add")
}

func isServerAlreadyInServerPool(s *serverPool, addr string) bool {
	for _, srv := range s.GetAllServers() {
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

func bootUpAdditionalServer(s *serverPool) (server.ServerInstance, error) {
	if len(s.GetActiveServers()) >= int(s.lbConfig.MaxNumOfServers) {
		return nil, fmt.Errorf("already max servers reached")
	}

	s.additionalServerMux.Lock()
	defer time.Sleep(2 * time.Second)
	defer s.additionalServerMux.Unlock()

	url, err := getAddrForNewServer(s)
	if err != nil {
		return nil, fmt.Errorf("failed to add new server: %v", err)
	}

	srv, err := s.AddServer(url)
	if err != nil {
		return nil, fmt.Errorf("failed to add new server: %v", err)
	}

	return srv, nil
}

func waitTillServerReady(url string) bool {
	endpoint := url + "/api/health"

	for i := 0; i < 10; i++ {
		client := &http.Client{
			Timeout: 1 * time.Second,
		}
		resp, err := client.Get(endpoint)
		if err == nil && resp.StatusCode == http.StatusOK {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

func (s *serverPool) LogServerPoolStatus(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		var msg string
		var msgDuration time.Time

		const (
			colorReset  = "\033[0m"
			colorGreen  = "\033[32m"
			colorRed    = "\033[31m"
			colorYellow = "\033[33m"
			colorBlue   = "\033[34m"
			colorCyan   = "\033[36m"

			boxTopLeft     = "╔"
			boxTopRight    = "╗"
			boxBottomLeft  = "╚"
			boxBottomRight = "╝"
			boxHorizontal  = "═"
			boxVertical    = "║"
			boxTeeRight    = "╠"
			boxTeeLeft     = "╣"
		)

		for {
			select {
			case <-ctx.Done():
				s.logger.Println("Gracefully shutting down serverpool")
				s.shutdownServerPool()
				return
			case newMsg := <-s.logChan:
				msg = newMsg
				msgDuration = time.Now().Add(3 * time.Second)
			case <-ticker.C:
				// pretty printing thanks to claude
				s.logger.Print("\033[H\033[2J")

				header := " Server Status Dashboard "
				borderWidth := 80
				padding := (borderWidth - len(header)) / 2

				s.logger.Printf("%s%s%s%s%s\n",
					boxTopLeft,
					strings.Repeat(boxHorizontal, padding),
					colorCyan+header+colorReset,
					strings.Repeat(boxHorizontal, padding),
					boxTopRight)

				if time.Now().Before(msgDuration) {
					s.logger.Printf("%s %s%s%s %s\n",
						boxVertical,
						colorYellow,
						msg,
						colorReset,
						boxVertical)
					s.logger.Printf("%s%s%s\n",
						boxTeeRight,
						strings.Repeat(boxHorizontal, borderWidth),
						boxTeeLeft)
				}

				for _, server := range s.servers {
					metrics := server.GetMetrics()
					healthStatus := colorGreen + "HEALTHY" + colorReset
					if !server.IsHealthy() {
						healthStatus = colorRed + "UNHEALTHY" + colorReset
					}

					s.logger.Printf("%s %-20s | %s | Active: %s%-4d%s | Failed: %s%-4d%s | Success: %s%-4d%s %s\n",
						boxVertical,
						server.GetUrl(),
						healthStatus,
						colorBlue,
						metrics.ActiveRequests,
						colorReset,
						colorRed,
						metrics.FailedRequests,
						colorReset,
						colorGreen,
						metrics.SuccessfulRequests,
						colorReset,
						boxVertical)
				}

				s.logger.Printf("%s%s%s\n",
					boxBottomLeft,
					strings.Repeat(boxHorizontal, borderWidth),
					boxBottomRight)

				s.logger.Printf("\nLast updated: %s%s%s\n",
					colorCyan,
					time.Now().Format("2006-01-02 15:04:05"),
					colorReset)
			}
		}
	}()
}

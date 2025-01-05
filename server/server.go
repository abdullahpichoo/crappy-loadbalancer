package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

type serverStatus string

const (
	alive serverStatus = "alive"
	dead  serverStatus = "dead"
)

type server struct {
	status    serverStatus
	url       url.URL
	process   *exec.Cmd
	metrics   ServerMetrics
	isHealthy bool
	mux       sync.RWMutex
	rp        httputil.ReverseProxy
}

type ServerMetrics struct {
	ActiveRequests     atomic.Int32
	FailedRequests     atomic.Int32
	SuccessfulRequests atomic.Int32
}

type ReadOnlyServerMetrics struct {
	ActiveRequests     int32
	FailedRequests     int32
	SuccessfulRequests int32
}

type ServerInstance interface {
	GetStatus() serverStatus
	IsAlive() bool
	GetUrl() string
	BootUp() (ServerInstance, error)
	Kill() error
	SetIsHealthy(bool)
	IsHealthy() bool
	GetMetrics() ReadOnlyServerMetrics
	GetReverseProxy() *httputil.ReverseProxy
	IncrementActiveRequests()
	DecrementActiveRequests()
	RecordRequestResult(bool)
}

func (s *server) IsAlive() bool {
	return s.status == alive
}

func (s *server) BootUp() (ServerInstance, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.status == alive {
		return nil, errors.New("server is already running")
	}

	cmd := exec.Command("./main", s.url.Port())
	cmd.Dir = "../backend"
	cmd.Stderr = nil
	cmd.Stdout = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	s.status = alive
	s.process = cmd
	return s, nil
}

func (s *server) Kill() error {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.status == dead {
		return fmt.Errorf("server already dead")
	}
	if err := s.process.Process.Kill(); err != nil {
		return err
	}
	s.status = dead
	return nil
}

func (s *server) SetIsHealthy(isHealthy bool) {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.isHealthy = isHealthy
}

func (s *server) IsHealthy() bool {
	s.mux.RLock()
	defer s.mux.RUnlock()

	return s.isHealthy
}

func (s *server) GetStatus() serverStatus {
	return s.status
}

func (s *server) GetUrl() string {
	return s.url.String()
}

func (s *server) GetMetrics() ReadOnlyServerMetrics {
	return ReadOnlyServerMetrics{
		ActiveRequests:     s.metrics.ActiveRequests.Load(),
		FailedRequests:     s.metrics.FailedRequests.Load(),
		SuccessfulRequests: s.metrics.SuccessfulRequests.Load(),
	}
}

func (s *server) GetReverseProxy() *httputil.ReverseProxy {
	return &s.rp
}

func (s *server) IncrementActiveRequests() {
	s.metrics.ActiveRequests.Add(1)
}

func (s *server) DecrementActiveRequests() {
	s.metrics.ActiveRequests.Add(-1)
}

func (s *server) RecordRequestResult(succeeded bool) {
	if succeeded {
		s.metrics.SuccessfulRequests.Add(1)
	} else {
		s.metrics.FailedRequests.Add(1)
	}
}

func New(serverUrl string) (ServerInstance, error) {
	parsedUrl, err := url.Parse(serverUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize server %s", err.Error())
	}

	srv := &server{
		url:       *parsedUrl,
		status:    dead,
		metrics:   ServerMetrics{},
		isHealthy: true,
		mux:       sync.RWMutex{},
	}

	reverseProxy := createReverseProxy(srv)
	srv.rp = *reverseProxy

	return srv, nil

}

func createReverseProxy(srv *server) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(&srv.url)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		srv.metrics.ActiveRequests.Add(1)
	}
	proxy.Transport = &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}
	proxy.ModifyResponse = srv.rpModifyResponse
	proxy.ErrorHandler = srv.rpErrorHandler

	return proxy
}

func (s *server) rpModifyResponse(res *http.Response) error {
	defer s.DecrementActiveRequests()

	s.RecordRequestResult(res.StatusCode >= 200 && res.StatusCode < 400)

	return nil
}

func (s *server) rpErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	defer s.DecrementActiveRequests()

	s.RecordRequestResult(false)
	http.Error(w, "An error occured while processing the request "+err.Error(), http.StatusInternalServerError)
}

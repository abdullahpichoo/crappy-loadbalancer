package serversstore

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/abdullahpichoo/crappy-loadbalancer/loadbalancer/server"
)

type serverMetrics struct {
	activeRequests     atomic.Int32
	failedRequests     atomic.Int32
	successfulRequests atomic.Int32
}

type serverState struct {
	instance server.ServerInstance
	metrics  *serverMetrics
}

type ServersStateStore struct {
	mux     sync.RWMutex
	servers map[uint16]serverState
}

func NewServersStateStore() *ServersStateStore {
	return &ServersStateStore{
		servers: make(map[uint16]serverState),
		mux:     sync.RWMutex{},
	}
}

func (store *ServersStateStore) AddServer(instance server.ServerInstance) error {
	store.mux.Lock()
	defer store.mux.Unlock()

	id := uint16(len(store.servers) + 1)
	if _, exists := store.servers[id]; exists {
		return fmt.Errorf("server with ID %d already exists", id)
	}
	fmt.Println("new server added in store ", id)
	store.servers[id] = serverState{
		instance: instance,
		metrics:  &serverMetrics{},
	}

	return nil
}

func (s *ServersStateStore) RemoveServer(id uint16) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	if _, exists := s.servers[id]; !exists {
		return fmt.Errorf("server with ID %d not found", id)
	}

	delete(s.servers, id)
	return nil
}

func (s *ServersStateStore) IncrementActiveRequests(id uint16) error {
	s.mux.RLock()
	server, exists := s.servers[id]
	s.mux.RUnlock()

	if !exists {
		return fmt.Errorf("server with ID %d not found", id)
	}

	server.metrics.activeRequests.Add(1)
	return nil
}

func (s *ServersStateStore) DecrementActiveRequests(id uint16) error {
	s.mux.RLock()
	server, exists := s.servers[id]
	s.mux.RUnlock()

	if !exists {
		return fmt.Errorf("server with ID %d not found", id)
	}

	server.metrics.activeRequests.Add(-1)
	return nil
}

func (s *ServersStateStore) RecordRequestResult(id uint16, succeeded bool) error {
	s.mux.RLock()
	server, exists := s.servers[id]
	s.mux.RUnlock()

	if !exists {
		return fmt.Errorf("server with ID %d not found", id)
	}

	if succeeded {
		server.metrics.successfulRequests.Add(1)
	} else {
		server.metrics.failedRequests.Add(1)
	}
	return nil
}

func (s *ServersStateStore) GetServerMetrics(id uint16) (*serverMetrics, error) {
	s.mux.RLock()
	server, exists := s.servers[id]
	s.mux.RUnlock()

	if !exists {
		return nil, fmt.Errorf("server with ID %d not found", id)
	}

	return server.metrics, nil
}

func (s *ServersStateStore) GetActiveServers() []uint16 {
	s.mux.RLock()
	defer s.mux.RUnlock()

	activeServers := make([]uint16, 0)
	for id, server := range s.servers {
		if server.instance.IsAlive() {
			activeServers = append(activeServers, id)
		}
	}
	return activeServers
}

type ReadOnlyServerStats struct {
	ActiveRequests     int32
	SuccessfulRequests int32
	FailedRequests     int32
}

func (s *ServersStateStore) GetServerStats(id uint16) (ReadOnlyServerStats, error) {
	s.mux.RLock()
	server, exists := s.servers[id]
	s.mux.RUnlock()

	if !exists {
		return ReadOnlyServerStats{}, fmt.Errorf("server with ID %d not found", id)
	}

	return ReadOnlyServerStats{
		ActiveRequests:     server.metrics.activeRequests.Load(),
		SuccessfulRequests: server.metrics.successfulRequests.Load(),
		FailedRequests:     server.metrics.failedRequests.Load(),
	}, nil
}

func (s *ServersStateStore) GetServerInstance(id uint16) (server.ServerInstance, error) {
	s.mux.RLock()
	server, exists := s.servers[id]
	s.mux.RUnlock()

	if !exists {
		return nil, fmt.Errorf("server with ID %d not found", id)
	}

	return server.instance, nil
}

func (s *ServersStateStore) FindServerByUrl(url string) (uint16, error) {
	s.mux.RLock()
	for srv := range s.servers {
		if s.servers[srv].instance.GetUrl() == url {
			return srv, nil
		}
	}
	return 0, errors.New("unable to find server with url " + url)
}

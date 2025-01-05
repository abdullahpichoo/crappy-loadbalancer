package healthcheck

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/abdullahpichoo/crappy-loadbalancer/loadbalancer/serverpool"
)

type HealthChecker struct {
	Client     *http.Client
	Interval   time.Duration
	ServerPool serverpool.ServerPool
}

func (hc *HealthChecker) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(hc.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				hc.checkAllServers()

			}
		}
	}()
}

func (hc *HealthChecker) checkAllServers() {
	for _, srv := range hc.ServerPool.GetActiveServers() {
		isHealthy := hc.healthCheck(srv.GetUrl())
		if !isHealthy {
			go hc.ServerPool.RestartServer(srv.GetUrl())
		}
	}
}

func (hc *HealthChecker) healthCheck(url string) bool {
	endpoint := url + "/api/health"

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		fmt.Printf("Failed to create health check request for %s: %v", url, err)
		return false
	}

	const successThreshold = 3
	successCount := 0

	for i := 0; i < successThreshold; i++ {
		resp, err := hc.Client.Do(req)
		if err != nil {
			hc.ServerPool.SendMessage(fmt.Sprintf("Health check failed for %s: %v", url, err))
			return false
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			hc.ServerPool.SendMessage(fmt.Sprintf("Unhealthy status code from %s: %d", url, resp.StatusCode))
			return false
		}
		successCount += 1
	}

	return successCount == successThreshold
}

func NewHealthChecker(pool serverpool.ServerPool, interval time.Duration, timeout time.Duration) *HealthChecker {
	return &HealthChecker{
		Client: &http.Client{
			Timeout: timeout,
		},
		Interval:   interval,
		ServerPool: pool,
	}
}

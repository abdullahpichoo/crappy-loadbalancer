package config

type LbConfig struct {
	MaxNumOfServers    uint16
	DefaultServerAddr  string
	InitialServerAddrs []string
	MaxConnsPerServer  uint16
	ServerAddrs        []string
}

package config

import (
	"encoding/json"
	"errors"
	"os"
)

type LbConfig struct {
	ServerAddrs        []string `json:"server_addrs"`
	InitialServerAddrs []string `json:"initial_server_addrs"`
	MaxConnsPerServer  uint16   `json:"max_conns_per_server"`
	MaxNumOfServers    uint16   `json:"max_num_of_servers"`
	Strategy           string   `json:"strategy"`
}

func GetConfig() (*LbConfig, error) {
	file, err := os.Open("config.json")
	if err != nil {
		return nil, errors.New("error opening the config.json")
	}
	defer file.Close()

	var config LbConfig
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, errors.New("error reading the config.json, please check format")
	}

	return &config, nil
}

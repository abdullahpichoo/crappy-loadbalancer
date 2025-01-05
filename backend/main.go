package main

import (
	"fmt"
	"log"
	"os"

	"github.com/abdullahpichoo/crappy-loadbalancer/backend/server"
)

func main() {
	var log log.Logger
	portNum := os.Args[1]
	addr := fmt.Sprintf("localhost:%s", portNum)

	if err := server.StartServer(addr, &log); err != nil {
		log.Fatalln("error starting server")
	}

}

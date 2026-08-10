package main

import (
	"log"

	"github.com/aetosdios27/shiden/internal/server"
)

func main() {
	shiden := server.Server{Address: server.DefaultAddress}
	if err := shiden.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

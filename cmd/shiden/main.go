package main

import (
	"log"

	"github.com/aetosdios27/shiden/internal/server"
)

func main() {
	shiden := server.New(server.DefaultAddress)
	if err := shiden.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

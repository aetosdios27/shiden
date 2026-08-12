package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/aetosdios27/shiden/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shiden := server.New(server.DefaultAddress)
	if err := shiden.ListenAndServe(ctx); err != nil {
		log.Fatal(err)
	}
	log.Print("Shiden stopped")
}

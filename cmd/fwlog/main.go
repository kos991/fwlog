package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"fwlog/internal/config"
	"fwlog/internal/server"
)

func main() {
	cfg := config.LoadConfig()
	app := server.NewApp(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fwlog: %v\n", err)
		os.Exit(1)
	}
}

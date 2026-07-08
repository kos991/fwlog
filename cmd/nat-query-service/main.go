package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	fwlog "nat-query-service/internal/app"
)

func main() {
	cfg := fwlog.LoadConfig()
	app := fwlog.NewApp(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "nat-query-service: %v\n", err)
		os.Exit(1)
	}
}

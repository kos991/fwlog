package main

import fwlog "nat-query-service/internal/app"

func main() {
	cfg := fwlog.LoadConfig()
	app := fwlog.NewApp(cfg)
	if err := app.Run(); err != nil {
		panic(err)
	}
}

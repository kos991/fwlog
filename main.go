package main

func main() {
	cfg := LoadConfig()
	app := NewApp(cfg)
	if err := app.Run(); err != nil {
		panic(err)
	}
}

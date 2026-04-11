package main

import (
	"log"
	stdhttp "net/http"

	"github.com/benenen/myclaw/internal/bootstrap"
	"github.com/benenen/myclaw/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	app, err := bootstrap.New(cfg)
	if err != nil {
		log.Fatalf("bootstrap app: %v", err)
	}

	if err := stdhttp.ListenAndServe(cfg.HTTPAddr, app.Handler); err != nil {
		log.Fatalf("run server: %v", err)
	}
}

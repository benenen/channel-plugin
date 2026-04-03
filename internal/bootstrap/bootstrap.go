package bootstrap

import (
	stdhttp "net/http"

	"github.com/benenen/channel-plugin/internal/config"
)

type App struct {
	Config  config.Config
	Handler stdhttp.Handler
}

func New(cfg config.Config) (*App, error) {
	mux := stdhttp.NewServeMux()
	mux.HandleFunc("/healthz", func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return &App{
		Config:  cfg,
		Handler: mux,
	}, nil
}

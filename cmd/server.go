package cmd

import (
	"io"
	"log"
	stdhttp "net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/benenen/myclaw/internal/bootstrap"
	"github.com/benenen/myclaw/internal/config"
)

var (
	loadConfig     = config.Load
	newApp         = bootstrap.New
	listenAndServe = stdhttp.ListenAndServe
)

func NewServerCommand(stderr io.Writer, exitCode *int) *cobra.Command {
	return NewServerCommandWithRunner(stderr, exitCode, RunServer)
}

func NewServerCommandWithRunner(stderr io.Writer, exitCode *int, runner func(io.Writer) int) *cobra.Command {
	return &cobra.Command{
		Use:   "server",
		Short: "Run the HTTP server",
		Run: func(_ *cobra.Command, _ []string) {
			*exitCode = runner(stderr)
		},
	}
}

func RunServer(stderr io.Writer) int {
	logger := log.New(stderr, "", log.LstdFlags)

	cfg, err := loadConfig()
	if err != nil {
		logger.Printf("load config: %v", err)
		return 1
	}

	app, err := newApp(cfg)
	if err != nil {
		logger.Printf("bootstrap app: %v", err)
		return 1
	}

	logger.Printf("web server listening on %s", serviceURL(cfg.HTTPAddr))
	if err := listenAndServe(cfg.HTTPAddr, app.Handler); err != nil {
		logger.Printf("run server: %v", err)
		return 1
	}

	return 0
}

func serviceURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	return "http://" + addr
}

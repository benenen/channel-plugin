package cmd

import (
	"fmt"
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

func Execute(args []string, stdout, stderr io.Writer) int {
	return executeWithServer(args, stdout, stderr, runServer)
}

func executeWithServer(args []string, stdout, stderr io.Writer, server func(io.Writer) int) int {
	root, exitCode := newRootCommand(stdout, stderr, server)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		if len(args) > 0 && !isHelpArg(args[0]) {
			for _, command := range root.Commands() {
				if command.Name() == args[0] {
					fmt.Fprintln(stderr, err)
					return 1
				}
			}

			fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
			writeUsage(stderr)
			return 1
		}

		fmt.Fprintln(stderr, err)
		return 1
	}

	return *exitCode
}

func newRootCommand(stdout, stderr io.Writer, server func(io.Writer) int) (*cobra.Command, *int) {
	exitCode := 0

	root := &cobra.Command{
		Use:           "myclaw",
		SilenceErrors: true,
		SilenceUsage:  true,
		Run: func(_ *cobra.Command, _ []string) {
			exitCode = server(stderr)
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetUsageFunc(func(cmd *cobra.Command) error {
		writeUsage(cmd.OutOrStdout())
		return nil
	})

	root.AddCommand(&cobra.Command{
		Use:   "server",
		Short: "Run the HTTP server",
		Run: func(_ *cobra.Command, _ []string) {
			exitCode = server(stderr)
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "notify <message>",
		Short: "Print a placeholder notification message",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), args[0])
		},
	})

	return root, &exitCode
}

func isHelpArg(arg string) bool {
	switch arg {
	case "help", "-h", "--help":
		return true
	default:
		return false
	}
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  myclaw [server]")
	fmt.Fprintln(w, "  myclaw notify <message>")
	fmt.Fprintln(w, "  myclaw help")
}

func runServer(stderr io.Writer) int {
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

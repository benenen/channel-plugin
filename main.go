package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithServer(args, stdout, stderr, func(io.Writer) int {
		return 1
	})
}

func runWithServer(args []string, stdout, stderr io.Writer, server func(io.Writer) int) int {
	if len(args) == 0 {
		return server(stderr)
	}

	switch args[0] {
	case "server":
		return server(stderr)
	case "help", "-h", "--help":
		writeUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		writeUsage(stderr)
		return 1
	}
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  myclaw [server]")
	fmt.Fprintln(w, "  myclaw help")
}

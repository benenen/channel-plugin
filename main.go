package main

import (
	"os"

	"github.com/benenen/myclaw/cmd"
)

func main() {
	os.Exit(cmd.Execute(os.Args[1:], os.Stdout, os.Stderr))
}

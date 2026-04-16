package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

func main() {
	cmd := exec.Command("/opt/homebrew/bin/codex")
	cmd.Env = os.Environ()

	ptmx, err := pty.Start(cmd)
	if err != nil {
		panic(err)
	}
	defer ptmx.Close()

	term := vt10x.New(vt10x.WithSize(120, 40))

	var raw bytes.Buffer
	done := make(chan error, 1)

	go func() {
		_, err := io.Copy(io.MultiWriter(term, &raw), ptmx)
		done <- err
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(20 * time.Second)

	for {
		select {
		case <-ticker.C:
			fmt.Print("\x1b[2J\x1b[H")
			fmt.Println("===== SCREEN =====")
			fmt.Println(term.String())

		case <-timeout:
			fmt.Print("\x1b[2J\x1b[H")
			fmt.Println("===== FINAL SCREEN =====")
			fmt.Println(term.String())

			fmt.Println("===== RAW =====")
			fmt.Println(raw.String())

			_ = ptmx.Close()

			select {
			case err := <-done:
				if err != nil && err != io.EOF {
					fmt.Printf("copy error: %v\n", err)
				}
			case <-time.After(2 * time.Second):
				fmt.Println("copy goroutine did not exit in time")
			}

			if cmd.Process != nil {
				_ = cmd.Process.Kill()
				_, _ = cmd.Process.Wait()
			}
			return
		}
	}
}

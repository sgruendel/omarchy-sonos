package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"omarchy-sonos/internal/app"
)

func main() {
	stateRoot := os.Getenv("XDG_STATE_HOME")
	if stateRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Cannot locate home directory")
			os.Exit(1)
		}
		stateRoot = filepath.Join(home, ".local", "state")
	}
	dir := flag.String("state-dir", filepath.Join(stateRoot, "sgruendel.sonos"), "directory for private state and RP session")
	hosts := flag.String("hosts", os.Getenv("SONOS_HOSTS"), "comma-separated speaker IPs (optional discovery seeds)")
	once := flag.Bool("once", false, "emit a read-only snapshot and exit")
	flag.Parse()
	a, err := app.New(*dir, app.ParseHosts(*hosts))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = a.Run(ctx, os.Stdin, os.Stdout, *once); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

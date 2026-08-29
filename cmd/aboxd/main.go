package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/daemon"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/version"
)

const name = "aboxd"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		if _, err := fmt.Fprintln(stdout, version.Current().String(name)); err != nil {
			return 1
		}
		return 0
	}
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	return serve(ctx, args, stderr)
}

func serve(ctx context.Context, args []string, stderr io.Writer) int {
	runtimeDir, stateDatabase, err := defaultPaths()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: %v\n", err)
		return contract.ExitPrerequisite
	}
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&runtimeDir, "runtime-dir", runtimeDir, "secure daemon runtime directory")
	flags.StringVar(&stateDatabase, "state-db", stateDatabase, "Agent Box state database")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return contract.ExitUsage
	}

	store, err := state.Open(ctx, stateDatabase)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: open state: %v\n", err)
		return contract.ExitInfrastructure
	}
	defer func() {
		if err := store.Close(); err != nil {
			_, _ = fmt.Fprintf(stderr, "aboxd: close state: %v\n", err)
		}
	}()
	operationJournal, err := journal.New(store)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: create operation journal: %v\n", err)
		return contract.ExitInternal
	}
	server, err := daemon.Start(daemon.Config{
		RuntimeDir: runtimeDir,
		Journal:    operationJournal,
		Run: func(context.Context, journal.Operation, json.RawMessage, daemon.Emit) *contract.Failure {
			return &contract.Failure{Code: contract.CodeFeatureNotSupported}
		},
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: start: %v\n", err)
		return contract.ExitInfrastructure
	}

	waitErr := server.Wait(ctx)
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: shutdown: %v\n", err)
		return contract.ExitInfrastructure
	}
	if waitErr != nil && !errors.Is(waitErr, context.Canceled) {
		_, _ = fmt.Fprintf(stderr, "aboxd: serve: %v\n", waitErr)
		return contract.ExitInfrastructure
	}
	return contract.ExitOK
}

func defaultPaths() (runtimeDir, stateDatabase string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve home directory: %w", err)
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		return "", "", fmt.Errorf("canonicalize home directory: %w", err)
	}
	if runtime.GOOS == "darwin" {
		applicationRoot := filepath.Join(home, "Library", "Application Support", "Agent Box")
		return filepath.Join(applicationRoot, "runtime"), filepath.Join(applicationRoot, "state.db"), nil
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}
	applicationRoot := filepath.Join(stateHome, "agent-box")
	runtimeDir = filepath.Join(applicationRoot, "runtime")
	if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
		runtimeDir = filepath.Join(xdgRuntime, "agent-box")
	}
	return runtimeDir, filepath.Join(applicationRoot, "state.db"), nil
}

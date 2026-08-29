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
	"syscall"
	"time"

	"github.com/shoe-point-0/agent-box/internal/boxlist"
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
	userPaths, err := daemon.DefaultUserPaths()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: %v\n", err)
		return contract.ExitPrerequisite
	}
	runtimeDir, stateDatabase := userPaths.RuntimeDir, userPaths.StateDatabase
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
	boxInventory, err := boxlist.New(store)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: create Box inventory: %v\n", err)
		return contract.ExitInternal
	}
	server, err := daemon.Start(daemon.Config{
		RuntimeDir: runtimeDir,
		Journal:    operationJournal,
		List:       boxInventory.List,
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

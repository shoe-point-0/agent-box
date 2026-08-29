package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/shoe-point-0/agent-box/internal/spike"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	registry := spike.NewRegistry()
	if err := spike.RegisterHarnessSmoke(registry); err != nil {
		fmt.Fprintf(stderr, "initialize spike registry: %v\n", err)
		return 1
	}
	if err := spike.RegisterGitPathProbe(registry); err != nil {
		fmt.Fprintf(stderr, "initialize Git path probe: %v\n", err)
		return 1
	}
	if err := spike.RegisterComposeProbe(registry); err != nil {
		fmt.Fprintf(stderr, "initialize Compose probe: %v\n", err)
		return 1
	}
	if err := spike.RegisterRoutingProbe(registry); err != nil {
		fmt.Fprintf(stderr, "initialize routing probe: %v\n", err)
		return 1
	}
	return runWithRegistry(context.Background(), args, stdout, stderr, registry)
}

func runWithRegistry(ctx context.Context, args []string, stdout, stderr io.Writer, registry *spike.Registry) int {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "list":
		flags := flag.NewFlagSet("list", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		jsonOutput := flags.Bool("json", false, "emit JSON")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			return usageError(stderr)
		}
		names := registry.Names()
		if *jsonOutput {
			return encodeJSON(stdout, stderr, struct {
				SchemaVersion string   `json:"schemaVersion"`
				Spikes        []string `json:"spikes"`
			}{SchemaVersion: "abox.spike.list/v1", Spikes: names})
		}
		for _, name := range names {
			fmt.Fprintln(stdout, name)
		}
		return 0
	case "run":
		flags := flag.NewFlagSet("run", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		parent := flags.String("parent", os.TempDir(), "parent directory for retained run roots")
		maxOutput := flags.Int("max-output", 1024*1024, "maximum captured stdout and stderr bytes per command")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 1 || *maxOutput <= 0 {
			return usageError(stderr)
		}
		result, runErr := spike.Execute(ctx, registry, flags.Arg(0), *parent, *maxOutput)
		if result.SchemaVersion == "" {
			fmt.Fprintf(stderr, "run spike: %v\n", runErr)
			return 1
		}
		if code := encodeJSON(stdout, stderr, result); code != 0 {
			return code
		}
		if runErr != nil {
			return 1
		}
		return 0
	case "inspect":
		if len(args) != 2 {
			return usageError(stderr)
		}
		inspection, err := spike.Inspect(args[1])
		if err != nil {
			fmt.Fprintf(stderr, "inspect spike root: %v\n", err)
			return 1
		}
		return encodeJSON(stdout, stderr, inspection)
	case "cleanup":
		if len(args) != 2 {
			return usageError(stderr)
		}
		if err := spike.Cleanup(ctx, registry, args[1]); err != nil {
			fmt.Fprintf(stderr, "cleanup spike root: %v\n", err)
			return 1
		}
		return encodeJSON(stdout, stderr, struct {
			SchemaVersion string `json:"schemaVersion"`
			Status        string `json:"status"`
			Root          string `json:"root"`
		}{SchemaVersion: "abox.spike.cleanup/v1", Status: "cleaned", Root: args[1]})
	default:
		return usageError(stderr)
	}
}

func encodeJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "encode output: %v\n", err)
		return 1
	}
	return 0
}

func usageError(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: abox-spike list [--json] | run [--parent DIR] [--max-output BYTES] <name> | inspect <root> | cleanup <root>")
	return 2
}

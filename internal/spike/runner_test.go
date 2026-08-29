package spike

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunnerBoundsOutputAndReportsExit(t *testing.T) {
	t.Parallel()

	result, err := (Runner{MaxOutput: 8}).Run(context.Background(), helperCommand("output"))
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("Run(output) error = %v, want ErrCommandFailed", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("Run(output) exit = %d, want 7", result.ExitCode)
	}
	if len(result.Stdout) != 8 || len(result.Stderr) != 8 || !result.StdoutTruncated || !result.StderrTruncated {
		t.Fatalf("Run(output) result = %+v, want independently truncated 8-byte streams", result)
	}
}

func TestRunnerPreservesLiteralArguments(t *testing.T) {
	t.Parallel()

	literal := "$(touch should-not-exist); *"
	command := helperCommand("literal", literal)
	result, err := (Runner{}).Run(context.Background(), command)
	if err != nil {
		t.Fatalf("Run(literal) error = %v", err)
	}
	if got := strings.TrimSpace(string(result.Stdout)); got != literal {
		t.Fatalf("Run(literal) stdout = %q, want %q", got, literal)
	}
}

func TestRunnerHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := (Runner{}).Run(ctx, helperCommand("sleep"))
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("Run(sleep) error = %v, want ErrCommandFailed", err)
	}
	if result.ExitCode != -1 {
		t.Fatalf("Run(sleep) exit = %d, want -1", result.ExitCode)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("Run(sleep) took %s after cancellation", elapsed)
	}
}

func helperCommand(mode string, arguments ...string) Command {
	argv := []string{os.Args[0], "-test.run=TestRunnerHelperProcess", "--", mode}
	argv = append(argv, arguments...)
	return Command{
		Argv: argv,
		Env:  append(os.Environ(), "ABOX_SPIKE_HELPER=1"),
	}
}

func TestRunnerHelperProcess(t *testing.T) {
	if os.Getenv("ABOX_SPIKE_HELPER") != "1" {
		return
	}
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator == -1 || separator+1 >= len(os.Args) {
		os.Exit(90)
	}
	arguments := os.Args[separator+1:]
	switch arguments[0] {
	case "output":
		fmt.Fprint(os.Stdout, strings.Repeat("o", 32))
		fmt.Fprint(os.Stderr, strings.Repeat("e", 32))
		os.Exit(7)
	case "literal":
		fmt.Fprintln(os.Stdout, arguments[1])
		os.Exit(0)
	case "sleep":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	default:
		os.Exit(91)
	}
}

package toolchain

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/shoe-point-0/agent-box/internal/process"
)

type DiagnosticProbe string

const (
	ProbeLimaVersion    DiagnosticProbe = "lima-version"
	ProbeDockerVersion  DiagnosticProbe = "docker-version"
	ProbeComposeVersion DiagnosticProbe = "compose-version"
	ProbeDockerRuntime  DiagnosticProbe = "docker-runtime"
)

type DiagnosticResult = process.Result

// Environment validation errors contain no supplied values and let doctor
// distinguish unmet prerequisites from internal observation failures.
var (
	ErrDiagnosticPath = errors.New("configure tool diagnostics: PATH must contain only non-empty, clean absolute directories without control characters")
	ErrDiagnosticHome = errors.New("configure tool diagnostics: HOME must be a non-empty, clean absolute directory without control characters")
)

// Diagnostics executes only fixed, bounded tool and runtime observations. It
// cannot download, install, provision, apply, or repair an artifact.
type Diagnostics struct {
	directory   string
	environment map[string]string
	runner      Runner
}

func NewDiagnostics(path, home, directory string) (*Diagnostics, error) {
	if !cleanDiagnosticPath(directory) {
		return nil, errors.New("configure tool diagnostics: working directory is invalid")
	}
	if !cleanDiagnosticPath(home) {
		return nil, ErrDiagnosticHome
	}
	if path == "" {
		return nil, ErrDiagnosticPath
	}
	for _, entry := range filepath.SplitList(path) {
		if !cleanDiagnosticPath(entry) {
			return nil, ErrDiagnosticPath
		}
	}
	return &Diagnostics{
		directory:   directory,
		environment: map[string]string{"PATH": path, "HOME": home, "LANG": "C", "LC_ALL": "C", "TZ": "UTC"},
		runner:      process.Runner{CaptureLimit: 16 * 1024},
	}, nil
}

func cleanDiagnosticPath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && strings.IndexFunc(path, unicode.IsControl) == -1
}

func (diagnostics *Diagnostics) Observe(ctx context.Context, probe DiagnosticProbe) (DiagnosticResult, error) {
	if diagnostics == nil {
		return process.Result{}, errors.New("tool diagnostics are nil")
	}
	var argv []string
	switch probe {
	case ProbeLimaVersion:
		argv = []string{"limactl", "--version"}
	case ProbeDockerVersion:
		argv = []string{"docker", "--version"}
	case ProbeComposeVersion:
		argv = []string{"docker", "compose", "version", "--short"}
	case ProbeDockerRuntime:
		argv = []string{"docker", "version", "--format", "{{.Client.Version}}/{{.Server.Version}}"}
	default:
		return process.Result{}, errors.New("tool diagnostic probe is unsupported")
	}
	return diagnostics.runner.Run(ctx, process.Command{Argv: argv, Dir: diagnostics.directory, Env: diagnostics.environment})
}

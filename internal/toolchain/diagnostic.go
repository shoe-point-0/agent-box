package toolchain

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

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

// Diagnostics executes only fixed, bounded tool and runtime observations. It
// cannot download, install, provision, apply, or repair an artifact.
type Diagnostics struct {
	directory   string
	environment map[string]string
	runner      Runner
}

func NewDiagnostics(path, home, directory string) (*Diagnostics, error) {
	if path == "" || home == "" || directory == "" || !filepath.IsAbs(home) || !filepath.IsAbs(directory) ||
		filepath.Clean(home) != home || filepath.Clean(directory) != directory || strings.ContainsAny(path+home+directory, "\x00\r\n") {
		return nil, errors.New("configure tool diagnostics: paths are invalid")
	}
	for _, entry := range filepath.SplitList(path) {
		if !filepath.IsAbs(entry) || filepath.Clean(entry) != entry {
			return nil, errors.New("configure tool diagnostics: PATH is invalid")
		}
	}
	return &Diagnostics{
		directory:   directory,
		environment: map[string]string{"PATH": path, "HOME": home, "LANG": "C", "LC_ALL": "C", "TZ": "UTC"},
		runner:      process.Runner{CaptureLimit: 16 * 1024},
	}, nil
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

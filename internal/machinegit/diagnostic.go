package machinegit

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/shoe-point-0/agent-box/internal/process"
)

type DiagnosticProbe string

const (
	ProbeVersion           DiagnosticProbe = "version"
	ProbeRelativeWorktrees DiagnosticProbe = "relative-worktrees"
)

// DiagnosticClient executes only fixed, non-repository Git observations under
// the hardened Machine Git profile. It never prepares configuration or writes
// a managed excludes file.
type DiagnosticClient struct {
	gitPath   string
	home      string
	path      string
	directory string
	executor  Executor
}

func NewDefaultDiagnosticClient(path, home, directory string) (*DiagnosticClient, error) {
	return NewDiagnosticClient(path, home, directory, process.Runner{CaptureLimit: 16 * 1024})
}

func NewDefaultDiagnosticClientAt(gitPath, path, home, directory string) (*DiagnosticClient, error) {
	return newDiagnosticClient(gitPath, path, home, directory, process.Runner{CaptureLimit: 16 * 1024})
}

func NewDiagnosticClient(path, home, directory string, executor Executor) (*DiagnosticClient, error) {
	gitPath, err := diagnosticExecutable(path, "git")
	if err != nil {
		return nil, err
	}
	return newDiagnosticClient(gitPath, path, home, directory, executor)
}

func newDiagnosticClient(gitPath, path, home, directory string, executor Executor) (*DiagnosticClient, error) {
	if err := validatePathList(path); err != nil {
		return nil, err
	}
	for field, value := range map[string]string{"Git executable": gitPath, "home": home, "working directory": directory} {
		if err := validateCanonicalPath(field, value); err != nil {
			return nil, err
		}
	}
	if executor == nil {
		return nil, refuse("executor", "is required")
	}
	info, err := os.Stat(gitPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return nil, refuse("Git executable", "must be an executable regular file")
	}
	return &DiagnosticClient{gitPath: gitPath, home: home, path: path, directory: directory, executor: executor}, nil
}

func (client *DiagnosticClient) Observe(ctx context.Context, probe DiagnosticProbe) (Result, error) {
	if client == nil || client.executor == nil {
		return process.Result{}, refuse("diagnostic", "client is incomplete")
	}
	var arguments []string
	switch probe {
	case ProbeVersion:
		arguments = []string{"--version"}
	case ProbeRelativeWorktrees:
		arguments = []string{"worktree", "add", "-h"}
	default:
		return process.Result{}, refuse("diagnostic probe", "is unsupported")
	}
	argv := []string{client.gitPath, "--no-pager", "--no-replace-objects"}
	for _, setting := range fixedConfig(Credentials{}, nullPath) {
		argv = append(argv, "-c", setting)
	}
	argv = append(argv, arguments...)
	return client.executor.Run(ctx, process.Command{
		Argv: argv,
		Dir:  client.directory,
		Env: map[string]string{
			"PATH":                   client.path,
			"HOME":                   client.home,
			"XDG_CONFIG_HOME":        client.home,
			"LANG":                   "C",
			"LC_ALL":                 "C",
			"TZ":                     "UTC",
			"GIT_CONFIG_GLOBAL":      nullPath,
			"GIT_CONFIG_SYSTEM":      nullPath,
			"GIT_CONFIG_NOSYSTEM":    "1",
			"GIT_CONFIG_COUNT":       "0",
			"GIT_ATTR_NOSYSTEM":      "1",
			"GIT_TERMINAL_PROMPT":    "0",
			"GIT_PROTOCOL_FROM_USER": "0",
			"GIT_NO_REPLACE_OBJECTS": "1",
			"GIT_OPTIONAL_LOCKS":     "0",
			"GIT_PAGER":              "cat",
			"PAGER":                  "cat",
			"GIT_EDITOR":             falsePath,
			"GIT_SEQUENCE_EDITOR":    falsePath,
		},
	})
}

func diagnosticExecutable(pathValue, name string) (string, error) {
	for _, directory := range filepath.SplitList(pathValue) {
		candidate := filepath.Join(directory, name)
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		canonical, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return "", refuse("Git executable", "cannot be canonicalized")
		}
		if err := validateCanonicalPath("Git executable", canonical); err != nil {
			return "", err
		}
		return canonical, nil
	}
	return "", errors.New("machine git diagnostic: executable not found")
}

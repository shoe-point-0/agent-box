package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// UserPaths are the user-scoped daemon endpoints shared by abox and aboxd.
type UserPaths struct {
	RuntimeDir    string
	StateDatabase string
	LogFile       string
	Socket        string
}

// DefaultUserPaths resolves the platform-native user layout without creating it.
func DefaultUserPaths() (UserPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return UserPaths{}, fmt.Errorf("resolve home directory: %w", err)
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		return UserPaths{}, fmt.Errorf("canonicalize home directory: %w", err)
	}
	home, err = cleanAbsolute("home directory", home)
	if err != nil {
		return UserPaths{}, err
	}

	if runtime.GOOS == "darwin" {
		applicationRoot := filepath.Join(home, "Library", "Application Support", "Agent Box")
		runtimeDir := filepath.Join(applicationRoot, "runtime")
		return UserPaths{
			RuntimeDir: runtimeDir, StateDatabase: filepath.Join(applicationRoot, "state.db"),
			LogFile: filepath.Join(home, "Library", "Logs", "Agent Box", "aboxd.log"),
			Socket:  runtimeSocketPath(runtimeDir),
		}, nil
	}

	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}
	stateHome, err = cleanAbsolute("XDG state directory", stateHome)
	if err != nil {
		return UserPaths{}, err
	}
	applicationRoot := filepath.Join(stateHome, "agent-box")
	runtimeDir := filepath.Join(applicationRoot, "runtime")
	if value := os.Getenv("XDG_RUNTIME_DIR"); value != "" {
		runtimeRoot, pathErr := cleanAbsolute("XDG runtime directory", value)
		if pathErr != nil {
			return UserPaths{}, pathErr
		}
		runtimeDir = filepath.Join(runtimeRoot, "agent-box")
	}
	return UserPaths{
		RuntimeDir: runtimeDir, StateDatabase: filepath.Join(applicationRoot, "state.db"),
		LogFile: filepath.Join(applicationRoot, "aboxd.log"), Socket: runtimeSocketPath(runtimeDir),
	}, nil
}

func cleanAbsolute(name, path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be absolute", name)
	}
	if filepath.Clean(path) != path {
		return "", fmt.Errorf("%s must be clean", name)
	}
	if filepath.Base(path) == string(filepath.Separator) {
		return "", errors.New(name + " cannot be the filesystem root")
	}
	return path, nil
}

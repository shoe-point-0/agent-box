package lima

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"

	"github.com/shoe-point-0/agent-box/internal/process"
)

const (
	maxGuestEnvironmentEntries = 256
	maxGuestEnvironmentBytes   = 1 << 20
)

// GuestCommand is a complete guest subprocess request. Environment is the
// entire child environment, not an overlay on the runtime host environment.
type GuestCommand struct {
	Argv        []string
	Workdir     string
	Environment map[string]string
	Secrets     []process.Secret
	Stdout      io.Writer
	Stderr      io.Writer
}

func compileGuestInvocation(command GuestCommand) ([]string, []byte, error) {
	if len(command.Argv) == 0 || command.Argv[0] == "" {
		return nil, nil, errors.New("guest argv is empty")
	}
	for _, argument := range command.Argv {
		if strings.IndexByte(argument, 0) >= 0 {
			return nil, nil, errors.New("guest argv contains NUL")
		}
		for _, secret := range command.Secrets {
			if secret.Value != "" && strings.Contains(argument, secret.Value) {
				return nil, nil, errors.New("registered secret appears in guest argv")
			}
		}
	}
	for _, secret := range command.Secrets {
		if secret.Value != "" && strings.Contains(command.Workdir, secret.Value) {
			return nil, nil, errors.New("registered secret appears in guest workdir")
		}
	}
	if len(command.Environment) > maxGuestEnvironmentEntries {
		return nil, nil, errors.New("guest environment has too many entries")
	}

	names := make([]string, 0, len(command.Environment))
	total := 0
	for name, value := range command.Environment {
		if !portableEnvironmentName(name) || strings.IndexByte(value, 0) >= 0 {
			return nil, nil, errors.New("guest environment is invalid")
		}
		total += len(name) + len(value)
		if total > maxGuestEnvironmentBytes {
			return nil, nil, errors.New("guest environment exceeds size limit")
		}
		names = append(names, name)
	}
	slices.Sort(names)

	var script strings.Builder
	script.WriteString("set -eu\nunset PWD SHLVL _\n")
	for _, name := range names {
		fmt.Fprintf(&script, "export %s=%s\n", name, shellQuote(command.Environment[name]))
	}
	script.WriteString("exec \"$@\"\n")
	if script.Len() > maxGuestEnvironmentBytes {
		return nil, nil, errors.New("guest environment transport exceeds size limit")
	}
	return append([]string{"/usr/bin/env", "-i", "/bin/sh", "-s", "--"}, command.Argv...), []byte(script.String()), nil
}

func portableEnvironmentName(name string) bool {
	for index, current := range name {
		if current > unicode.MaxASCII || (current != '_' && !unicode.IsLetter(current) && (index == 0 || !unicode.IsDigit(current))) {
			return false
		}
	}
	return name != ""
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func cloneSecrets(input []process.Secret) []process.Secret {
	return slices.Clone(input)
}

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
	maxGuestTransportBytes     = 4 << 20
	// MaxGuestInputBytes bounds all generated input files in one invocation.
	MaxGuestInputBytes = 1 << 20
)

// GuestInputFile replaces one argv element with a private temporary guest file.
// Its contents travel over stdin and are removed when the command exits.
type GuestInputFile struct {
	ArgIndex int
	Data     []byte
}

// GuestCommand is a complete guest subprocess request. Environment is the
// entire child environment, not an overlay on the runtime host environment.
type GuestCommand struct {
	InputFiles  []GuestInputFile
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
	if len(command.InputFiles) == 0 {
		for _, name := range names {
			fmt.Fprintf(&script, "export %s=%s\n", name, shellQuote(command.Environment[name]))
		}
		script.WriteString("exec \"$@\"\n")
	} else {
		if len(command.InputFiles) > 16 {
			return nil, nil, errors.New("too many guest input files")
		}
		inputs := make(map[int][]byte, len(command.InputFiles))
		size := 0
		for _, file := range command.InputFiles {
			if file.ArgIndex <= 0 || file.ArgIndex >= len(command.Argv) {
				return nil, nil, errors.New("guest input argument index is invalid")
			}
			if _, exists := inputs[file.ArgIndex]; exists {
				return nil, nil, errors.New("duplicate guest input argument index")
			}
			size += len(file.Data)
			if size > MaxGuestInputBytes || strings.IndexByte(string(file.Data), 0) >= 0 {
				return nil, nil, errors.New("guest input files exceed bounds or contain NUL")
			}
			inputs[file.ArgIndex] = file.Data
		}
		script.WriteString("umask 077\nstage=$(/usr/bin/mktemp -d /tmp/abox-inputs.XXXXXXXXXX)\ntrap '/bin/rm -rf -- \"$stage\"' EXIT\ntrap 'exit 130' HUP INT TERM\n")
		for index := range command.Argv {
			if data, ok := inputs[index]; ok {
				fmt.Fprintf(&script, "printf '%%s' %s > \"$stage/%d\"\n", shellQuote(string(data)), index)
			}
		}
		// Keep transport variables out of the requested child environment.
		script.WriteString("/usr/bin/env -i")
		for _, name := range names {
			fmt.Fprintf(&script, " %s", shellQuote(name+"="+command.Environment[name]))
		}
		script.WriteByte(' ')
		for index, arg := range command.Argv {
			if index > 0 {
				script.WriteByte(' ')
			}
			if _, ok := inputs[index]; ok {
				fmt.Fprintf(&script, "\"$stage/%d\"", index)
			} else {
				script.WriteString(shellQuote(arg))
			}
		}
		script.WriteByte('\n')
	}
	if script.Len() > maxGuestTransportBytes {
		return nil, nil, errors.New("guest command transport exceeds size limit")
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

package compose

import (
	"context"
	"errors"

	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/process"
)

// GuestCommandInvoker transports a complete command to one runtime host.
type GuestCommandInvoker interface {
	InvokeCommand(context.Context, lima.Spec, lima.GuestCommand) (process.Result, error)
}

// GuestRunner binds normalization to one freshly reconciled runtime host.
type GuestRunner struct {
	invoker GuestCommandInvoker
	spec    lima.Spec
}

func NewGuestRunner(invoker GuestCommandInvoker, spec lima.Spec) (*GuestRunner, error) {
	if invoker == nil || spec.RuntimeHost.InstanceName == "" {
		return nil, errors.New("configure Compose guest runner: invoker and runtime host are required")
	}
	return &GuestRunner{invoker: invoker, spec: spec}, nil
}

func (r *GuestRunner) Run(ctx context.Context, command Command) (CommandResult, error) {
	secrets := make([]process.Secret, len(command.Secrets))
	for index, secret := range command.Secrets {
		secrets[index] = process.Secret{Value: secret.Value}
	}
	result, err := r.invoker.InvokeCommand(ctx, r.spec, lima.GuestCommand{
		Argv: append([]string(nil), command.Argv...), Workdir: command.Dir,
		Environment: cloneMap(command.Env), Secrets: secrets,
	})
	return CommandResult{
		Stdout: append([]byte(nil), result.Stdout...), Stderr: append([]byte(nil), result.Stderr...),
		StdoutTruncated: result.StdoutTruncated, StderrTruncated: result.StderrTruncated,
	}, err
}

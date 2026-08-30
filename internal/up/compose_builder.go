package up

import (
	"context"
	"errors"

	"github.com/shoe-point-0/agent-box/internal/compose"
)

// NewComposeConfigBuilder creates a concurrency-safe delayed builder. Every
// invocation binds a new normalization runner to that attempt's freshly
// observed runtime host instead of retaining mutable Box identity.
func NewComposeConfigBuilder(invoker compose.GuestCommandInvoker, dockerPath string, environment map[string]string, secrets []compose.Secret) (BuildComposeConfig, error) {
	if invoker == nil || !cleanAbsoluteGeneratedPath(dockerPath) {
		return nil, errors.New("configure Compose builder: invoker and absolute Docker path are required")
	}
	configuredEnvironment := cloneStringMap(environment)
	configuredSecrets := append([]compose.Secret(nil), secrets...)
	return func(_ context.Context, _ PreparedAttempt, foundation FoundationResult, guest GuestResult) (ComposeConfig, error) {
		if guest.Spec.DockerPath != dockerPath || guest.Spec.RuntimeHost.RuntimeHost.InstanceName != foundation.Lima.Spec.RuntimeHost.InstanceName {
			return ComposeConfig{}, errors.New("configure Compose builder: verified guest identity differs from runtime host")
		}
		runner, err := compose.NewGuestRunner(invoker, foundation.Lima.Spec)
		if err != nil {
			return ComposeConfig{}, err
		}
		return ComposeConfig{
			Runner: runner, DockerPath: dockerPath,
			Environment: cloneStringMap(configuredEnvironment), Secrets: append([]compose.Secret(nil), configuredSecrets...),
		}, nil
	}, nil
}

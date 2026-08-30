package up

import (
	"context"
	"reflect"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/process"
)

func TestComposeConfigBuilderBindsEachFreshRuntimeWithoutSharedIdentity(t *testing.T) {
	invoker := &builderGuestInvoker{}
	environment := map[string]string{"HOME": "/runtime/home"}
	secrets := []compose.Secret{{Value: "secret-canary"}}
	builder, err := NewComposeConfigBuilder(invoker, "/usr/bin/docker", environment, secrets)
	if err != nil {
		t.Fatal(err)
	}
	environment["HOME"] = "mutated"
	secrets[0].Value = "mutated"

	for _, name := range []string{"box-one", "box-two"} {
		foundation := FoundationResult{Lima: LimaResult{Spec: lima.Spec{
			RuntimeHost: contract.RuntimeHostPlan{InstanceName: name},
		}}}
		config, err := builder(t.Context(), PreparedAttempt{}, foundation)
		if err != nil {
			t.Fatal(err)
		}
		if config.DockerPath != "/usr/bin/docker" || config.Environment["HOME"] != "/runtime/home" || config.Secrets[0].Value != "secret-canary" {
			t.Fatalf("built Compose config = %#v", config)
		}
		if _, err := config.Runner.Run(t.Context(), compose.Command{Argv: []string{"/usr/bin/docker", "compose", "version"}, Dir: "/runtime"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := invoker.names; !reflect.DeepEqual(got, []string{"box-one", "box-two"}) {
		t.Fatalf("runtime host bindings = %q", got)
	}
}

type builderGuestInvoker struct {
	names []string
}

func (i *builderGuestInvoker) InvokeCommand(_ context.Context, spec lima.Spec, _ lima.GuestCommand) (process.Result, error) {
	i.names = append(i.names, spec.RuntimeHost.InstanceName)
	return process.Result{}, nil
}

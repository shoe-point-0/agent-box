package spike

import (
	"context"
	"os"
)

const HarnessSmoke = "harness-smoke"

func RegisterHarnessSmoke(registry *Registry) error {
	return registry.Register(HarnessSmoke, func(ctx context.Context, env *Environment) error {
		if _, err := env.WriteFile("smoke.txt", []byte("harness-only\n")); err != nil {
			return err
		}
		_, err := env.RunCommand(ctx, "go-version", Command{
			Argv: []string{"go", "version"},
			Env:  os.Environ(),
		})
		return err
	})
}

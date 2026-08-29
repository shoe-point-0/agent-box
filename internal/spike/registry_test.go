package spike

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRegistryStartsEmpty(t *testing.T) {
	t.Parallel()

	if names := NewRegistry().Names(); len(names) != 0 {
		t.Fatalf("Names() = %v, want empty", names)
	}
}

func TestRegistryRegistersRunsAndSortsNames(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	var gotRoot string
	if err := registry.Register("zeta", func(context.Context, *Environment) error { return nil }); err != nil {
		t.Fatalf("Register(zeta) error = %v", err)
	}
	if err := registry.Register("alpha", func(_ context.Context, env *Environment) error {
		gotRoot = env.Root()
		return nil
	}); err != nil {
		t.Fatalf("Register(alpha) error = %v", err)
	}

	if got, want := registry.Names(), []string{"alpha", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	if err := registry.Run(context.Background(), "alpha", &Environment{root: "/tmp/probe-root"}); err != nil {
		t.Fatalf("Run(alpha) error = %v", err)
	}
	if gotRoot != "/tmp/probe-root" {
		t.Fatalf("runner root = %q, want %q", gotRoot, "/tmp/probe-root")
	}
}

func TestRegistryRejectsInvalidAndDuplicateEntries(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	if err := registry.Register("", func(context.Context, *Environment) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Register(empty) error = %v, want ErrInvalid", err)
	}
	if err := registry.Register("probe", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Register(nil) error = %v, want ErrInvalid", err)
	}
	if err := registry.Register("probe", func(context.Context, *Environment) error { return nil }); err != nil {
		t.Fatalf("Register(probe) error = %v", err)
	}
	if err := registry.Register("probe", func(context.Context, *Environment) error { return nil }); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("Register(duplicate) error = %v, want ErrDuplicate", err)
	}
}

func TestRegistryReportsUnknownSpike(t *testing.T) {
	t.Parallel()

	err := NewRegistry().Run(context.Background(), "missing", &Environment{root: t.TempDir()})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Run(missing) error = %v, want ErrNotFound", err)
	}
}

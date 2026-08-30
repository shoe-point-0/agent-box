package doctor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/doctor"
)

type observerFunc func(context.Context, doctor.Request) ([]doctor.Check, error)

func (function observerFunc) Observe(ctx context.Context, request doctor.Request) ([]doctor.Check, error) {
	return function(ctx, request)
}

func TestInspectSortsAndPreservesEveryEvidenceClass(t *testing.T) {
	checks := []doctor.Check{
		{Result: "z.unknown", Evidence: doctor.EvidenceUnknown, Status: doctor.StatusAdvisory, Observed: "not observed", Required: "live fact", Remediation: "observe it later"},
		{Result: "a.tested", Evidence: doctor.EvidenceTested, Status: doctor.StatusPass, Observed: "present", Required: "present"},
		{Result: "m.warning", Evidence: doctor.EvidenceWarning, Status: doctor.StatusAdvisory, Observed: "conditional", Required: "measured support", Remediation: "run the hardware gate"},
		{Result: "n.unsupported", Evidence: doctor.EvidenceUnsupported, Status: doctor.StatusFail, Observed: "missing", Required: "supported capability", Remediation: "install the capability"},
	}
	report, err := doctor.Inspect(t.Context(), doctor.Request{Mode: doctor.ModeFilesystem, TaskKey: "TASK-1"}, observerFunc(func(context.Context, doctor.Request) ([]doctor.Check, error) {
		return checks, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != doctor.SchemaVersion || report.Mode != doctor.ModeFilesystem || report.TaskKey != "TASK-1" {
		t.Fatalf("report identity = %#v", report)
	}
	if len(report.Checks) != 4 || report.Checks[0].Result != "a.tested" || report.Checks[3].Result != "z.unknown" {
		t.Fatalf("checks are not stable and sorted: %#v", report.Checks)
	}
	if !report.Failed() {
		t.Fatal("unsupported failed prerequisite did not fail the report")
	}
}

func TestInspectRejectsAmbiguousObservations(t *testing.T) {
	valid := doctor.Check{Result: "machine.test", Evidence: doctor.EvidenceTested, Status: doctor.StatusPass, Observed: "yes", Required: "yes"}
	for _, test := range []struct {
		name   string
		checks []doctor.Check
	}{
		{name: "empty"},
		{name: "duplicate", checks: []doctor.Check{valid, valid}},
		{name: "missing remediation", checks: []doctor.Check{{Result: "machine.test", Evidence: doctor.EvidenceTested, Status: doctor.StatusFail, Observed: "no", Required: "yes"}}},
		{name: "ambiguous warning", checks: []doctor.Check{{Result: "machine.test", Evidence: doctor.EvidenceWarning, Status: doctor.StatusPass, Observed: "maybe", Required: "yes"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := doctor.Inspect(t.Context(), doctor.Request{Mode: doctor.ModeMachine}, observerFunc(func(context.Context, doctor.Request) ([]doctor.Check, error) {
				return test.checks, nil
			}))
			if err == nil {
				t.Fatal("Inspect() accepted ambiguous observation")
			}
		})
	}
}

func TestInspectHonorsCancellationBeforeObservation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	_, err := doctor.Inspect(ctx, doctor.Request{Mode: doctor.ModeMachine}, observerFunc(func(context.Context, doctor.Request) ([]doctor.Check, error) {
		calls++
		return nil, nil
	}))
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("Inspect() error/calls = %v/%d", err, calls)
	}
}

func TestLocalObserverReportsModesWithoutMutationDependencies(t *testing.T) {
	observer, err := doctor.NewLocalObserver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []doctor.Request{
		{Mode: doctor.ModeMachine},
		{Mode: doctor.ModeBox, TaskKey: "TASK-1"},
		{Mode: doctor.ModeFilesystem, TaskKey: "TASK-1"},
	} {
		report, err := doctor.Inspect(t.Context(), request, observer)
		if err != nil {
			t.Fatalf("Inspect(%s) error = %v", request.Mode, err)
		}
		seen := make(map[string]doctor.Check, len(report.Checks))
		for _, check := range report.Checks {
			seen[check.Result] = check
		}
		for _, result := range []string{"disk.runtime", "disk.state", "git.relative-worktrees", "machine.ownership", "machine.platform", "port.http-router", "runtime.docker", "tool.artifact-integrity", "tool.compose", "tool.docker", "tool.git", "tool.lima"} {
			if _, ok := seen[result]; !ok {
				t.Errorf("%s report lacks %s", request.Mode, result)
			}
		}
		if request.Mode != doctor.ModeMachine {
			for _, result := range []string{"box.mounts", "box.ownership", "box.ports", "box.runtime"} {
				if seen[result].Evidence != doctor.EvidenceUnknown {
					t.Errorf("%s %s = %#v, want honest unknown", request.Mode, result, seen[result])
				}
			}
		}
		if request.Mode == doctor.ModeFilesystem {
			if seen["filesystem.watcher"].Evidence != doctor.EvidenceWarning || seen["filesystem.probe"].Evidence != doctor.EvidenceWarning {
				t.Fatalf("filesystem report lacks polling/probe guidance: %#v", seen)
			}
		}
	}
}

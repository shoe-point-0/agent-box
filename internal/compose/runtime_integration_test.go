package compose_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	aboxprocess "github.com/shoe-point-0/agent-box/internal/process"
)

func TestRuntimeLifecycleAgainstRealDocker(t *testing.T) {
	docker, version := realCompose(t)
	if err := exec.Command(docker, "image", "inspect", "alpine:3.20").Run(); err != nil {
		t.Skipf("cached lifecycle fixture image alpine:3.20 is unavailable: %v", err)
	}
	compiled, outputDirectory, _ := fixturePlan(t, version)
	project := compiled.Compose.ProjectName
	assertNoComposeProjectResources(t, docker, project)

	fixture := `services:
  workspace:
    image: alpine:3.20
    command: ["sh", "-c", "trap 'exit 0' TERM; while :; do echo runtime-tick; sleep 0.05; done"]
    volumes:
      - runtime-data:/data
  worker:
    image: alpine:3.20
    command: ["sleep", "infinity"]
volumes:
  runtime-data: {}
`
	if err := os.WriteFile(compiled.Compose.Includes[0].Path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	compilerRunner := processAdapter{runner: aboxprocess.Runner{CaptureLimit: composeModelLimitForTest}}
	generated, err := compose.Compile(t.Context(), compose.Input{
		Plan: compiled, Directory: outputDirectory, Generation: generation,
		ComposeCommand: []string{docker, "compose"}, Environment: processEnvironment(), Runner: compilerRunner,
	})
	if err != nil {
		t.Fatalf("Compile(runtime fixture) error = %v", err)
	}

	runtime, err := compose.NewRuntime(hostRuntimeInvoker{runner: aboxprocess.Runner{CaptureLimit: composeModelLimitForTest}})
	if err != nil {
		t.Fatal(err)
	}
	spec := compose.RuntimeSpec{
		Plan: compiled, Generation: generation, DockerPath: docker, Workdir: outputDirectory, Compiled: generated,
	}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			args := []string{"compose", "--project-name", project, "-f", generated.RootPath, "-f", generated.OverridePath, "down", "--volumes", "--remove-orphans"}
			_ = exec.Command(docker, args...).Run()
		}
	})

	first, err := runtime.Apply(t.Context(), spec)
	if err != nil {
		t.Fatalf("Apply(first) error = %v", err)
	}
	second, err := runtime.Apply(t.Context(), spec)
	if err != nil {
		t.Fatalf("Apply(second) error = %v", err)
	}
	if !slices.Equal(containerIDs(first), containerIDs(second)) || !slices.Equal(networkIDs(first), networkIDs(second)) || !slices.Equal(volumeIDs(first), volumeIDs(second)) {
		t.Fatalf("second apply changed resource IDs: first=%+v second=%+v", first, second)
	}
	if len(second.Containers) != 2 || len(second.Networks) != 1 || len(second.Volumes) != 1 || len(second.Images) != 1 {
		t.Fatalf("observation counts = containers %d networks %d volumes %d images %d", len(second.Containers), len(second.Networks), len(second.Volumes), len(second.Images))
	}
	for _, container := range second.Containers {
		if len(container.Ports) != 0 || container.Labels[ownership.ComposeBoxIDFact] != compiled.BoxID || container.ImageID == "" {
			t.Fatalf("container observation is unsafe or unattributed: %+v", container)
		}
	}
	if second.Networks[0].Labels[ownership.ComposeBoxIDFact] != compiled.BoxID || second.Volumes[0].Labels[ownership.ComposeBoxIDFact] != compiled.BoxID {
		t.Fatalf("network or volume lacks Box attribution: %+v %+v", second.Networks[0], second.Volumes[0])
	}
	if !slices.Equal(second.Images[0].ReferencedBy, containerIDs(second)) {
		t.Fatalf("image references = %q, want container IDs %q", second.Images[0].ReferencedBy, containerIDs(second))
	}

	result, err := runtime.Exec(t.Context(), spec, "workspace", []string{"sh", "-c", "exit 37"}, nil, nil)
	var exitErr *aboxprocess.ExitError
	if result.ExitCode != 37 || !errors.As(err, &exitErr) || exitErr.Code != 37 {
		t.Fatalf("Exec(exit 37) = result %+v error %v", result, err)
	}

	logsContext, cancelLogs := context.WithCancel(t.Context())
	logs := newNotifyingBuffer()
	logsDone := make(chan error, 1)
	go func() {
		_, logsErr := runtime.Logs(logsContext, spec, "workspace", true, 1, logs, io.Discard)
		logsDone <- logsErr
	}()
	select {
	case <-logs.written:
	case <-time.After(3 * time.Second):
		cancelLogs()
		t.Fatal("Logs did not stream output before deadline")
	}
	cancelLogs()
	select {
	case logsErr := <-logsDone:
		var runtimeErr *compose.RuntimeError
		if !errors.As(logsErr, &runtimeErr) || runtimeErr.Kind != compose.RuntimeCanceled {
			t.Fatalf("Logs(cancel) error = %v, want RuntimeCanceled", logsErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Logs did not stop after cancellation")
	}
	if !strings.Contains(logs.String(), "runtime-tick") {
		t.Fatalf("streamed logs = %q, want runtime output", logs.String())
	}

	stopped, err := runtime.Stop(t.Context(), spec)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	for _, container := range stopped.Containers {
		if container.State != "exited" {
			t.Fatalf("stopped container state = %q, want exited", container.State)
		}
	}
	records := compose.RecordsFromObservation(stopped)
	forged := records
	forged.Resources = append([]ownership.ComposeResourceRecord(nil), records.Resources...)
	forged.Resources[0].Labels = cloneLabels(forged.Resources[0].Labels)
	forged.Resources[0].Labels[ownership.ComposeBoxIDFact] = "forged-box"
	if _, removeErr := runtime.Remove(t.Context(), spec, forged); !errors.Is(removeErr, ownership.ErrNotOwned) {
		t.Fatalf("Remove(forged) error = %v, want ownership refusal", removeErr)
	}
	stillThere, err := runtime.Observe(t.Context(), spec)
	if err != nil || len(stillThere.Containers) != 2 {
		t.Fatalf("resources changed after ownership refusal: observation=%+v error=%v", stillThere, err)
	}

	after, err := runtime.Remove(t.Context(), spec, records)
	if err != nil {
		t.Fatalf("Remove(owned) error = %v", err)
	}
	if len(after.Containers)+len(after.Networks)+len(after.Volumes) != 0 {
		t.Fatalf("resources remain after Remove: %+v", after)
	}
	cleaned = true
	assertNoComposeProjectResources(t, docker, project)
}

type hostRuntimeInvoker struct {
	runner aboxprocess.Runner
}

func (i hostRuntimeInvoker) Invoke(ctx context.Context, _ lima.Spec, workdir string, argv []string, stdout, stderr io.Writer) (aboxprocess.Result, error) {
	return i.runner.Run(ctx, aboxprocess.Command{
		Argv: argv, Dir: workdir, Env: processEnvironment(), Stdout: stdout, Stderr: stderr,
	})
}

func processEnvironment() map[string]string {
	environment := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			environment[name] = value
		}
	}
	return environment
}

func containerIDs(observation compose.RuntimeObservation) []string {
	ids := make([]string, 0, len(observation.Containers))
	for _, container := range observation.Containers {
		ids = append(ids, container.ID)
	}
	slices.Sort(ids)
	return ids
}

func networkIDs(observation compose.RuntimeObservation) []string {
	ids := make([]string, 0, len(observation.Networks))
	for _, network := range observation.Networks {
		ids = append(ids, network.ID)
	}
	slices.Sort(ids)
	return ids
}

func volumeIDs(observation compose.RuntimeObservation) []string {
	ids := make([]string, 0, len(observation.Volumes))
	for _, volume := range observation.Volumes {
		ids = append(ids, volume.ID)
	}
	slices.Sort(ids)
	return ids
}

func cloneLabels(labels map[string]string) map[string]string {
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}

type notifyingBuffer struct {
	mu      sync.Mutex
	buffer  bytes.Buffer
	written chan struct{}
	once    sync.Once
}

func newNotifyingBuffer() *notifyingBuffer {
	return &notifyingBuffer{written: make(chan struct{})}
}

func (b *notifyingBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written, err := b.buffer.Write(data)
	if written > 0 {
		b.once.Do(func() { close(b.written) })
	}
	return written, err
}

func (b *notifyingBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

package spike

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const resultSchema = "abox.spike.result/v1"

type Environment struct {
	root      string
	workspace *Workspace
	runner    Runner
}

func (e *Environment) Root() string {
	if e == nil {
		return ""
	}
	return e.root
}

func (e *Environment) WriteFile(name string, data []byte) (string, error) {
	if e == nil || e.workspace == nil {
		return "", errors.New("write spike file: environment is not initialized")
	}
	return e.workspace.WriteOwnedFile(name, data, 0o600)
}

func (e *Environment) Record(resource Resource) (InventoryEntry, error) {
	if e == nil || e.workspace == nil {
		return InventoryEntry{}, errors.New("record spike resource: environment is not initialized")
	}
	if resource.OwnerID == "" {
		resource.OwnerID = e.workspace.metadata.ID
	}
	return e.workspace.inventory.Append("created", resource)
}

func (e *Environment) RunCommand(ctx context.Context, label string, command Command) (CommandResult, error) {
	if e == nil || e.workspace == nil {
		return CommandResult{}, errors.New("run spike command: environment is not initialized")
	}
	result, runErr := e.runner.Run(ctx, command)
	metadata, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		return result, fmt.Errorf("encode command result: %w", marshalErr)
	}
	metadata = append(metadata, '\n')
	diagnostics := []struct {
		name string
		data []byte
	}{
		{name: label + ".json", data: metadata},
		{name: label + ".stdout", data: result.Stdout},
		{name: label + ".stderr", data: result.Stderr},
	}
	for _, diagnostic := range diagnostics {
		if _, err := e.workspace.WriteDiagnostic(diagnostic.name, diagnostic.data); err != nil {
			return result, fmt.Errorf("persist command diagnostic: %w", err)
		}
	}
	return result, runErr
}

type RunResult struct {
	SchemaVersion  string    `json:"schemaVersion"`
	Scenario       string    `json:"scenario"`
	Status         string    `json:"status"`
	Root           string    `json:"root"`
	Inventory      string    `json:"inventory"`
	DiagnosticsDir string    `json:"diagnosticsDir"`
	StartedAt      time.Time `json:"startedAt"`
	FinishedAt     time.Time `json:"finishedAt"`
	Error          string    `json:"error,omitempty"`
}

func Execute(ctx context.Context, registry *Registry, scenario, parent string, maxOutput int) (RunResult, error) {
	started := time.Now().UTC()
	workspace, err := NewWorkspace(parent, scenario)
	if err != nil {
		return RunResult{}, err
	}
	environment := &Environment{
		root:      workspace.Root(),
		workspace: workspace,
		runner:    Runner{MaxOutput: maxOutput},
	}
	runErr := registry.Run(ctx, scenario, environment)
	result := RunResult{
		SchemaVersion:  resultSchema,
		Scenario:       scenario,
		Status:         "passed",
		Root:           workspace.Root(),
		Inventory:      workspace.InventoryPath(),
		DiagnosticsDir: workspace.DiagnosticsPath(),
		StartedAt:      started,
		FinishedAt:     time.Now().UTC(),
	}
	if runErr != nil {
		result.Status = "failed"
		result.Error = runErr.Error()
	}
	if err := workspace.WriteResult(result); err != nil {
		if runErr != nil {
			return result, fmt.Errorf("scenario failed: %v; write result: %w", runErr, err)
		}
		return result, err
	}
	return result, runErr
}

type Inspection struct {
	SchemaVersion string           `json:"schemaVersion"`
	Metadata      RootMetadata     `json:"metadata"`
	Inventory     []InventoryEntry `json:"inventory"`
	Result        *RunResult       `json:"result,omitempty"`
}

func Inspect(path string) (Inspection, error) {
	workspace, err := OpenWorkspace(path)
	if err != nil {
		return Inspection{}, err
	}
	entries, err := workspace.Entries()
	if err != nil {
		return Inspection{}, err
	}
	inspection := Inspection{
		SchemaVersion: "abox.spike.inspection/v1",
		Metadata:      workspace.Metadata(),
		Inventory:     entries,
	}
	data, err := os.ReadFile(filepath.Join(workspace.Root(), resultFile))
	if err == nil {
		var result RunResult
		if err := json.Unmarshal(data, &result); err != nil {
			return Inspection{}, fmt.Errorf("inspect result: %w", err)
		}
		inspection.Result = &result
	} else if !errors.Is(err, os.ErrNotExist) {
		return Inspection{}, fmt.Errorf("inspect result: %w", err)
	}
	return inspection, nil
}

func Cleanup(ctx context.Context, registry *Registry, path string) error {
	workspace, err := OpenWorkspace(path)
	if err != nil {
		return err
	}
	entries, err := workspace.Entries()
	if err != nil {
		return err
	}
	active := make(map[string]InventoryEntry)
	for _, entry := range entries {
		active[entry.Resource.Kind+"\x00"+entry.Resource.ID] = entry
	}
	resources := make([]Resource, 0, len(active))
	for _, entry := range active {
		if entry.Action != "removed" {
			resources = append(resources, entry.Resource)
		}
	}
	if err := registry.Cleanup(ctx, workspace.metadata.Scenario, workspace, resources); err != nil {
		return err
	}
	return CleanupWorkspace(path)
}

// Package contract defines Agent Box's versioned machine-readable payloads.
//
// These types are the single ownership boundary shared by plan compilation,
// daemon transport, CLI rendering, and context generation. They contain
// references and observed facts, never secret values.
package contract

import (
	"encoding/json"
	"time"
)

const (
	PlanSchemaVersion       = "abox.plan/v1alpha1"
	ContextSchemaVersion    = "abox.context/v1alpha1"
	OutputSchemaVersion     = "abox.output/v1alpha1"
	ListSchemaVersion       = "abox.list/v1alpha1"
	PlanOutputSchemaVersion = "abox.plan-output/v1alpha1"
	EventSchemaVersion      = "abox.event/v1alpha1"
	ErrorSchemaVersion      = "abox.error/v1alpha1"
	DaemonAPIVersion        = "abox.api/v1alpha1"
)

// Digest identifies canonical content using an algorithm-prefixed digest.
type Digest string

// Plan is the immutable Milestone 1 input to one reconciliation attempt.
// Later-milestone resources are deliberately absent until an adapter consumes
// them; adding them is an explicit schema evolution.
type Plan struct {
	SchemaVersion    string               `json:"schemaVersion"`
	WorkspaceID      string               `json:"workspaceId"`
	BoxID            string               `json:"boxId"`
	TaskKey          string               `json:"taskKey"`
	Manifest         Artifact             `json:"manifest"`
	Repositories     []PlanRepository     `json:"repositories"`
	RuntimeHost      RuntimeHostPlan      `json:"runtimeHost"`
	Compose          ComposePlan          `json:"compose"`
	WorkspaceService WorkspaceServicePlan `json:"workspaceService"`
	Mounts           []MountPlan          `json:"mounts"`
	Policy           PolicyPlan           `json:"policy"`
	Tools            ToolVersions         `json:"tools"`
	PlanDigest       Digest               `json:"planDigest"`
}

// Artifact records the canonical path and content digest of a Plan input.
type Artifact struct {
	Path   string `json:"path"`
	Digest Digest `json:"digest"`
}

type PlanRepository struct {
	Name         string `json:"name"`
	SourcePath   string `json:"sourcePath"`
	BaseCommit   string `json:"baseCommit"`
	Branch       string `json:"branch"`
	WorktreePath string `json:"worktreePath"`
	GitCommonDir string `json:"gitCommonDir"`
}

type RuntimeHostPlan struct {
	Driver        string `json:"driver"`
	InstanceName  string `json:"instanceName"`
	CPUMillicores int64  `json:"cpuMillicores"`
	MemoryBytes   int64  `json:"memoryBytes"`
	DiskBytes     int64  `json:"diskBytes"`
}

type ComposePlan struct {
	ProjectName string     `json:"projectName"`
	Includes    []Artifact `json:"includes"`
	Profiles    []string   `json:"profiles"`
}

type WorkspaceServicePlan struct {
	Name             string `json:"name"`
	Repository       string `json:"repository"`
	WorkingDirectory string `json:"workingDirectory"`
}

// MountPlan is one exact canonical path admitted to the runtime host.
type MountPlan struct {
	Path     string   `json:"path"`
	Services []string `json:"services"`
	Writable bool     `json:"writable"`
}

// PolicyPlan contains behavior-affecting policy already validated by the
// manifest loader. Runtime adapters consume this immutable projection rather
// than reaching back into mutable manifest intent.
type PolicyPlan struct {
	GitMetadata   string `json:"gitMetadata"`
	DockerSocket  string `json:"dockerSocket"`
	MachineMounts string `json:"machineMounts"`
	NetworkEgress string `json:"networkEgress"`
}

// ToolVersions records behavior-affecting tool versions without host secrets.
type ToolVersions struct {
	AgentBox string `json:"agentBox"`
	Git      string `json:"git"`
	Lima     string `json:"lima"`
	Docker   string `json:"docker"`
	Compose  string `json:"compose"`
}

// Context is the READY-generation document exposed to humans and agents.
type Context struct {
	SchemaVersion    string              `json:"schemaVersion"`
	Box              BoxContext          `json:"box"`
	Workspace        WorkspaceContext    `json:"workspace"`
	Repositories     []ContextRepository `json:"repositories"`
	WorkspaceService WorkspaceService    `json:"workspaceService"`
	Services         []ServiceContext    `json:"services"`
	Routes           []RouteContext      `json:"routes"`
	Ports            []PortContext       `json:"ports"`
	Commands         []CommandContext    `json:"commands"`
	Policy           PolicyContext       `json:"policy"`
	Durability       string              `json:"durability"`
}

type BoxContext struct {
	ID         string `json:"id"`
	TaskKey    string `json:"taskKey"`
	State      string `json:"state"`
	Generation int64  `json:"generation"`
	PlanDigest Digest `json:"planDigest"`
}

type WorkspaceContext struct {
	ID           string `json:"id"`
	ManifestPath string `json:"manifestPath"`
	Root         string `json:"root"`
}

type ContextRepository struct {
	Name       string `json:"name"`
	Branch     string `json:"branch"`
	BaseCommit string `json:"baseCommit"`
	Path       string `json:"path"`
}

type WorkspaceService struct {
	Name             string `json:"name"`
	WorkingDirectory string `json:"workingDirectory"`
}

type ServiceContext struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

type RouteContext struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Ready bool   `json:"ready"`
}

type PortContext struct {
	Name  string `json:"name"`
	Host  string `json:"host"`
	Port  int    `json:"port"`
	Ready bool   `json:"ready"`
}

type CommandContext struct {
	Name string   `json:"name"`
	Argv []string `json:"argv"`
}

type PolicyContext struct {
	GitMetadata   string `json:"gitMetadata"`
	DockerSocket  string `json:"dockerSocket"`
	MachineMounts string `json:"machineMounts"`
	NetworkEgress string `json:"networkEgress"`
}

// Output is a non-streaming CLI command's versioned final result.
type Output struct {
	SchemaVersion string             `json:"schemaVersion"`
	OperationID   string             `json:"operationId"`
	Box           OutputBox          `json:"box"`
	Repositories  []OutputRepository `json:"repositories"`
	Routes        []RouteContext     `json:"routes"`
	Ports         []PortContext      `json:"ports"`
}

type OutputBox struct {
	ID            string `json:"id"`
	TaskKey       string `json:"taskKey"`
	State         string `json:"state"`
	WorkspaceRoot string `json:"workspaceRoot"`
	Generation    int64  `json:"generation"`
	PlanDigest    Digest `json:"planDigest"`
}

type OutputRepository struct {
	Name   string `json:"name"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
	Path   string `json:"path"`
	Dirty  bool   `json:"dirty"`
}

// ListOutput is the stable, non-streaming inventory returned by abox list.
// Boxes are ordered by ID and drift contains observations, never repair intent.
type ListOutput struct {
	SchemaVersion string    `json:"schemaVersion"`
	Boxes         []ListBox `json:"boxes"`
}

type ListBox struct {
	ID            string   `json:"id"`
	WorkspaceID   string   `json:"workspaceId"`
	TaskKey       string   `json:"taskKey"`
	ManifestPath  string   `json:"manifestPath"`
	State         string   `json:"state"`
	RecordedState string   `json:"recordedState"`
	Drift         []string `json:"drift"`
}

// PlanOutput is the stable read-only result of compiling desired intent and
// comparing it with currently retained and observed Box state.
type PlanOutput struct {
	SchemaVersion  string          `json:"schemaVersion"`
	Plan           Plan            `json:"plan"`
	Classification string          `json:"classification"`
	Reason         string          `json:"reason"`
	Operations     []PlanOperation `json:"operations"`
}

// PlanOperation describes proposed work and its safety implications. Plan
// rendering never executes an operation.
type PlanOperation struct {
	Action      string `json:"action"`
	Resource    string `json:"resource"`
	Reason      string `json:"reason"`
	Description string `json:"description"`
	Restart     bool   `json:"restart"`
	Destructive bool   `json:"destructive"`
	Conflict    bool   `json:"conflict"`
}

// Event is a streaming CLI command's versioned NDJSON envelope. Details retain
// the daemon's command-specific, already-redacted payload without reinterpretation.
type Event struct {
	SchemaVersion string          `json:"schemaVersion"`
	OperationID   string          `json:"operationId"`
	Sequence      int64           `json:"sequence"`
	Kind          string          `json:"kind"`
	Details       json.RawMessage `json:"details"`
	OccurredAt    time.Time       `json:"occurredAt"`
}

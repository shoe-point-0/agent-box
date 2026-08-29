// Package manifest loads and validates Agent Box Workspace manifests.
package manifest

// Workspace is the complete v1alpha1 syntax model. Canonical and normalized
// fields are loader output and are never decoded from YAML.
type Workspace struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Metadata   Metadata      `yaml:"metadata"`
	Spec       WorkspaceSpec `yaml:"spec"`
}

type Metadata struct {
	Name string `yaml:"name"`
}

type WorkspaceSpec struct {
	Repositories     []Repository     `yaml:"repositories"`
	Host             Host             `yaml:"host"`
	Compose          Compose          `yaml:"compose"`
	WorkspaceService WorkspaceService `yaml:"workspaceService"`
	Mounts           []Mount          `yaml:"mounts"`
	Caches           []Cache          `yaml:"caches"`
	Setup            []SetupJob       `yaml:"setup"`
	Routes           []Route          `yaml:"routes"`
	TCPPorts         []TCPPort        `yaml:"tcpPorts"`
	Secrets          []Secret         `yaml:"secrets"`
	Policy           Policy           `yaml:"policy"`
}

type Repository struct {
	Name           string   `yaml:"name"`
	Source         Source   `yaml:"source"`
	Checkout       Checkout `yaml:"checkout"`
	WorkspacePath  string   `yaml:"workspacePath"`
	CanonicalLocal string   `yaml:"-"`
}

type Source struct {
	Local  string `yaml:"local"`
	Remote string `yaml:"remote"`
}

type Checkout struct {
	Ref    string `yaml:"ref"`
	Branch string `yaml:"branch"`
}

type Host struct {
	CPUs          Scalar `yaml:"cpus"`
	Memory        Scalar `yaml:"memory"`
	Disk          Scalar `yaml:"disk"`
	CPUMillicores int64  `yaml:"-"`
	MemoryBytes   int64  `yaml:"-"`
	DiskBytes     int64  `yaml:"-"`
}

// Scalar accepts either a YAML string or number and preserves its text.
type Scalar string

func (s *Scalar) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	*s = Scalar(value)
	return nil
}

type Compose struct {
	Includes []ComposeInclude `yaml:"includes"`
	Profiles []string         `yaml:"profiles"`
}

type ComposeInclude struct {
	Repository string `yaml:"repository"`
	Path       string `yaml:"path"`
}

type WorkspaceService struct {
	Name             string `yaml:"name"`
	Repository       string `yaml:"repository"`
	WorkingDirectory string `yaml:"workingDirectory"`
}

type Mount struct {
	Repository string   `yaml:"repository"`
	Services   []string `yaml:"services"`
	Mode       string   `yaml:"mode"`
}

type Cache struct {
	Name    string `yaml:"name"`
	Service string `yaml:"service"`
	Target  string `yaml:"target"`
}

type SetupJob struct {
	Name             string   `yaml:"name"`
	Service          string   `yaml:"service"`
	Command          []string `yaml:"command"`
	RerunWhenChanged []string `yaml:"rerunWhenChanged"`
	After            []string `yaml:"after"`
	Rerun            string   `yaml:"rerun"`
}

type Route struct {
	Name       string    `yaml:"name"`
	Service    string    `yaml:"service"`
	TargetPort int       `yaml:"targetPort"`
	Readiness  Readiness `yaml:"readiness"`
}

type Readiness struct {
	HTTP         *HTTPReadiness `yaml:"http"`
	Timeout      Scalar         `yaml:"timeout"`
	TimeoutNanos int64          `yaml:"-"`
}

type HTTPReadiness struct {
	Path           string `yaml:"path"`
	ExpectedStatus int    `yaml:"expectedStatus"`
}

type TCPPort struct {
	Name       string `yaml:"name"`
	Service    string `yaml:"service"`
	TargetPort int    `yaml:"targetPort"`
	Allocate   string `yaml:"allocate"`
}

type Secret struct {
	Name                string         `yaml:"name"`
	FromHostEnvironment string         `yaml:"fromHostEnvironment"`
	ExposeTo            []SecretTarget `yaml:"exposeTo"`
}

type SecretTarget struct {
	Service string `yaml:"service"`
	As      string `yaml:"as"`
}

type Policy struct {
	GitMetadata   string `yaml:"gitMetadata"`
	DockerSocket  string `yaml:"dockerSocket"`
	MachineMounts string `yaml:"machineMounts"`
	NetworkEgress string `yaml:"networkEgress"`
}

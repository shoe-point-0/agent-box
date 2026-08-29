package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	DefaultMaxInputBytes      = 1 << 20
	DefaultMaxNesting         = 32
	DefaultMaxCollectionItems = 256
	DefaultMaxPathBytes       = 4096
	DefaultMaxTemplateOutput  = 4096
)

// Limits bounds all attacker-controlled parser dimensions.
type Limits struct {
	MaxInputBytes      int64
	MaxNesting         int
	MaxCollectionItems int
	MaxPathBytes       int
	MaxTemplateOutput  int
}

func DefaultLimits() Limits {
	return Limits{
		MaxInputBytes:      DefaultMaxInputBytes,
		MaxNesting:         DefaultMaxNesting,
		MaxCollectionItems: DefaultMaxCollectionItems,
		MaxPathBytes:       DefaultMaxPathBytes,
		MaxTemplateOutput:  DefaultMaxTemplateOutput,
	}
}

// FieldError is a stable, value-free manifest diagnostic.
type FieldError struct {
	Path    string
	Problem string
}

func (e *FieldError) Error() string { return e.Path + ": " + e.Problem }

// Loader owns parser limits and the explicit roots local repositories may use.
type Loader struct {
	limits Limits
	roots  []string
}

func NewLoader(allowedSourceRoots []string, limits Limits) (*Loader, error) {
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if len(allowedSourceRoots) == 0 {
		return nil, errors.New("manifest loader requires at least one allowed source root")
	}
	roots := make([]string, 0, len(allowedSourceRoots))
	for index, root := range allowedSourceRoots {
		canonical, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, fmt.Errorf("allowed source root %d: canonicalize: %w", index, err)
		}
		canonical, err = filepath.Abs(canonical)
		if err != nil {
			return nil, fmt.Errorf("allowed source root %d: absolute path: %w", index, err)
		}
		roots = append(roots, filepath.Clean(canonical))
	}
	return &Loader{limits: limits, roots: roots}, nil
}

func validateLimits(limits Limits) error {
	if limits.MaxInputBytes <= 0 || limits.MaxNesting <= 0 || limits.MaxCollectionItems <= 0 || limits.MaxPathBytes <= 0 || limits.MaxTemplateOutput <= 0 {
		return errors.New("manifest limits must all be positive")
	}
	return nil
}

// Load reads, strictly decodes, validates, and canonicalizes one manifest.
func (l *Loader) Load(path string) (*Workspace, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, l.limits.MaxInputBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read manifest: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close manifest: %w", closeErr)
	}
	if int64(len(data)) > l.limits.MaxInputBytes {
		return nil, &FieldError{Path: "$", Problem: "input exceeds configured byte limit"}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("manifest absolute path: %w", err)
	}
	return l.LoadBytes(data, filepath.Dir(absolute))
}

// LoadBytes loads a manifest using manifestDir as the base for local sources.
func (l *Loader) LoadBytes(data []byte, manifestDir string) (*Workspace, error) {
	if int64(len(data)) > l.limits.MaxInputBytes {
		return nil, &FieldError{Path: "$", Problem: "input exceeds configured byte limit"}
	}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return nil, &FieldError{Path: "$", Problem: "malformed YAML"}
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, &FieldError{Path: "$", Problem: "must be a mapping"}
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, &FieldError{Path: "$", Problem: "malformed YAML"}
		}
		return nil, &FieldError{Path: "$", Problem: "multiple YAML documents are forbidden"}
	}
	if err := l.inspectNode(document.Content[0], reflect.TypeFor[Workspace](), "$", 1); err != nil {
		return nil, err
	}

	var workspace Workspace
	if err := document.Content[0].Decode(&workspace); err != nil {
		return nil, &FieldError{Path: "$", Problem: "manifest value has an invalid type"}
	}
	if err := l.validate(&workspace, manifestDir); err != nil {
		return nil, err
	}
	return &workspace, nil
}

func (l *Loader) inspectNode(node *yaml.Node, expected reflect.Type, path string, depth int) error {
	if depth > l.limits.MaxNesting {
		return &FieldError{Path: path, Problem: "nesting exceeds configured limit"}
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return &FieldError{Path: path, Problem: "YAML aliases and anchors are forbidden"}
	}
	for expected.Kind() == reflect.Pointer {
		expected = expected.Elem()
	}
	switch expected.Kind() {
	case reflect.Struct:
		if node.Kind != yaml.MappingNode {
			return &FieldError{Path: path, Problem: "must be a mapping"}
		}
		if len(node.Content)/2 > l.limits.MaxCollectionItems {
			return &FieldError{Path: path, Problem: "mapping exceeds configured item limit"}
		}
		fields := yamlFields(expected)
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return &FieldError{Path: path, Problem: "mapping keys must be strings"}
			}
			fieldPath := joinField(path, key.Value)
			fieldType, ok := fields[key.Value]
			if !ok {
				return &FieldError{Path: fieldPath, Problem: "unknown field"}
			}
			if _, duplicate := seen[key.Value]; duplicate {
				return &FieldError{Path: fieldPath, Problem: "duplicate field"}
			}
			seen[key.Value] = struct{}{}
			if err := l.inspectNode(value, fieldType, fieldPath, depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if node.Kind != yaml.SequenceNode {
			return &FieldError{Path: path, Problem: "must be a sequence"}
		}
		if len(node.Content) > l.limits.MaxCollectionItems {
			return &FieldError{Path: path, Problem: "sequence exceeds configured item limit"}
		}
		for index, child := range node.Content {
			if err := l.inspectNode(child, expected.Elem(), path+"["+strconv.Itoa(index)+"]", depth+1); err != nil {
				return err
			}
		}
	default:
		if node.Kind != yaml.ScalarNode {
			return &FieldError{Path: path, Problem: "must be a scalar"}
		}
		switch expected.Kind() {
		case reflect.String:
			if expected.Name() != "Scalar" && node.Tag != "!!str" {
				return &FieldError{Path: path, Problem: "must be a string"}
			}
		case reflect.Int:
			if node.Tag != "!!int" {
				return &FieldError{Path: path, Problem: "must be an integer"}
			}
			if _, err := strconv.ParseInt(node.Value, 0, strconv.IntSize); err != nil {
				return &FieldError{Path: path, Problem: "integer is out of range"}
			}
		}
	}
	return nil
}

func yamlFields(value reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, value.NumField())
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		name := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if name == "" {
			name = field.Name
		}
		if name != "-" {
			fields[name] = field.Type
		}
	}
	return fields
}

func joinField(parent, child string) string {
	if parent == "$" {
		return child
	}
	return parent + "." + child
}

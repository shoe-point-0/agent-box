// Package identity derives stable Workspace and Box identifiers.
package identity

import (
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	workspacePrefix = "ws_"
	digestLength    = 8
	maxTaskKeyBytes = 128
	maxSlugBytes    = 63
)

var base32Encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// WorkspaceID identifies one canonical Workspace manifest location.
type WorkspaceID string

// String returns the serialized Workspace ID.
func (id WorkspaceID) String() string { return string(id) }

// BoxID is the deterministic display and lifecycle ID for a Workspace task.
type BoxID string

// String returns the serialized Box ID.
func (id BoxID) String() string { return string(id) }

// Workspace derives a Workspace ID from an already-canonical manifest path.
// The caller must resolve symlinks before calling Workspace.
func Workspace(canonicalManifestPath string) (WorkspaceID, error) {
	if err := validateCanonicalAbsolutePath(canonicalManifestPath); err != nil {
		return "", fmt.Errorf("derive workspace identity: %w", err)
	}

	return WorkspaceID(workspacePrefix + digest([]byte(canonicalManifestPath))), nil
}

// Box derives the specified Box display ID from a Workspace ID and task key.
func Box(workspaceID WorkspaceID, taskKey string) (BoxID, error) {
	if err := ValidateWorkspaceID(workspaceID); err != nil {
		return "", fmt.Errorf("derive box identity: %w", err)
	}
	if err := ValidateTaskKey(taskKey); err != nil {
		return "", fmt.Errorf("derive box identity: %w", err)
	}

	hashInput := make([]byte, 0, len(workspaceID)+1+len(taskKey))
	hashInput = append(hashInput, workspaceID...)
	hashInput = append(hashInput, 0)
	hashInput = append(hashInput, taskKey...)

	return BoxID(taskSlug(taskKey) + "-" + digest(hashInput)), nil
}

// ValidateWorkspaceID rejects values outside the generated Workspace ID form.
func ValidateWorkspaceID(id WorkspaceID) error {
	value := string(id)
	if len(value) != len(workspacePrefix)+digestLength || !strings.HasPrefix(value, workspacePrefix) {
		return errors.New("workspace ID has invalid form")
	}
	if !isBase32Digest(value[len(workspacePrefix):]) {
		return errors.New("workspace ID has invalid digest")
	}
	return nil
}

// ValidateBoxID rejects values outside the generated Box ID form.
func ValidateBoxID(id BoxID) error {
	value := string(id)
	separator := strings.LastIndexByte(value, '-')
	if separator < 1 || len(value)-separator-1 != digestLength {
		return errors.New("box ID has invalid form")
	}
	if !isSlug(value[:separator]) || !isBase32Digest(value[separator+1:]) {
		return errors.New("box ID has invalid form")
	}
	return nil
}

// ValidateTaskKey checks the path-safe v0.1 task-key contract.
func ValidateTaskKey(taskKey string) error {
	if taskKey == "" {
		return errors.New("task key is empty")
	}
	if len(taskKey) > maxTaskKeyBytes {
		return fmt.Errorf("task key exceeds %d bytes", maxTaskKeyBytes)
	}
	if !isASCIIAlphanumeric(taskKey[0]) || !isASCIIAlphanumeric(taskKey[len(taskKey)-1]) {
		return errors.New("task key must start and end with an ASCII letter or digit")
	}
	for index := range len(taskKey) {
		character := taskKey[index]
		if isASCIIAlphanumeric(character) || character == '-' || character == '_' || character == '.' {
			continue
		}
		return fmt.Errorf("task key contains invalid byte at offset %d", index)
	}
	return nil
}

func validateCanonicalAbsolutePath(path string) error {
	if path == "" {
		return errors.New("canonical manifest path is empty")
	}
	if strings.IndexByte(path, 0) >= 0 {
		return errors.New("canonical manifest path contains NUL")
	}
	if !filepath.IsAbs(path) {
		return errors.New("canonical manifest path is not absolute")
	}
	if filepath.Clean(path) != path {
		return errors.New("canonical manifest path is not clean")
	}
	if filepath.Base(path) == string(filepath.Separator) {
		return errors.New("canonical manifest path does not name a file")
	}
	return nil
}

func taskSlug(taskKey string) string {
	var builder strings.Builder
	builder.Grow(min(len(taskKey), maxSlugBytes))
	previousSeparator := false
	for index := range len(taskKey) {
		character := taskKey[index]
		if character >= 'A' && character <= 'Z' {
			character += 'a' - 'A'
		}
		if character == '-' || character == '_' || character == '.' {
			if !previousSeparator && builder.Len() < maxSlugBytes {
				builder.WriteByte('-')
			}
			previousSeparator = true
			continue
		}
		if builder.Len() < maxSlugBytes {
			builder.WriteByte(character)
		}
		previousSeparator = false
	}
	return strings.TrimRight(builder.String(), "-")
}

func digest(input []byte) string {
	sum := sha256.Sum256(input)
	return strings.ToLower(base32Encoding.EncodeToString(sum[:]))[:digestLength]
}

func isBase32Digest(value string) bool {
	if len(value) != digestLength {
		return false
	}
	for index := range len(value) {
		character := value[index]
		if (character < 'a' || character > 'z') && (character < '2' || character > '7') {
			return false
		}
	}
	return true
}

func isSlug(value string) bool {
	if value == "" || len(value) > maxSlugBytes || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for index := range len(value) {
		character := value[index]
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlphanumeric(character byte) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}

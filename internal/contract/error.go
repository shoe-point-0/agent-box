package contract

import "slices"

// Exit codes are the stable process contract from THE_BEGINNING.md section 14.3.
const (
	ExitOK             = 0
	ExitUsage          = 2
	ExitPrerequisite   = 3
	ExitConflict       = 4
	ExitInfrastructure = 5
	ExitReadiness      = 6
	ExitSafety         = 7
	ExitNotFound       = 8
	ExitCompatibility  = 9
	ExitInternal       = 10
)

// ErrorCode is a stable symbolic error identity. Published values are never
// repurposed; callers branch on these values rather than message text.
type ErrorCode string

const (
	CodeCLIUsage                  ErrorCode = "ABOX_CLI_USAGE"
	CodeManifestInvalid           ErrorCode = "ABOX_MANIFEST_INVALID"
	CodeFeatureNotSupported       ErrorCode = "ABOX_FEATURE_NOT_SUPPORTED"
	CodeMachineUnsupported        ErrorCode = "ABOX_MACHINE_UNSUPPORTED"
	CodePrerequisiteFailed        ErrorCode = "ABOX_PREREQUISITE_FAILED"
	CodeOwnershipConflict         ErrorCode = "ABOX_OWNERSHIP_CONFLICT"
	CodeIdentityConflict          ErrorCode = "ABOX_IDENTITY_CONFLICT"
	CodeBranchConflict            ErrorCode = "ABOX_BRANCH_CONFLICT"
	CodeOperationConflict         ErrorCode = "ABOX_OPERATION_CONFLICT"
	CodeDaemonUnavailable         ErrorCode = "ABOX_DAEMON_UNAVAILABLE"
	CodeInfrastructureApplyFailed ErrorCode = "ABOX_INFRASTRUCTURE_APPLY_FAILED"
	CodeServiceNotReady           ErrorCode = "ABOX_SERVICE_NOT_READY"
	CodeRouteNotReady             ErrorCode = "ABOX_ROUTE_NOT_READY"
	CodeGitDirtySource            ErrorCode = "ABOX_GIT_DIRTY_SOURCE"
	CodeCleanupRefused            ErrorCode = "ABOX_CLEANUP_REFUSED"
	CodeNotFound                  ErrorCode = "ABOX_NOT_FOUND"
	CodeAPIIncompatible           ErrorCode = "ABOX_API_INCOMPATIBLE"
	CodeInternalInvariant         ErrorCode = "ABOX_INTERNAL_INVARIANT"
)

var exitCodes = map[ErrorCode]int{
	CodeCLIUsage:                  ExitUsage,
	CodeManifestInvalid:           ExitUsage,
	CodeFeatureNotSupported:       ExitUsage,
	CodeMachineUnsupported:        ExitPrerequisite,
	CodePrerequisiteFailed:        ExitPrerequisite,
	CodeOwnershipConflict:         ExitConflict,
	CodeIdentityConflict:          ExitConflict,
	CodeBranchConflict:            ExitConflict,
	CodeOperationConflict:         ExitConflict,
	CodeDaemonUnavailable:         ExitInfrastructure,
	CodeInfrastructureApplyFailed: ExitInfrastructure,
	CodeServiceNotReady:           ExitReadiness,
	CodeRouteNotReady:             ExitReadiness,
	CodeGitDirtySource:            ExitSafety,
	CodeCleanupRefused:            ExitSafety,
	CodeNotFound:                  ExitNotFound,
	CodeAPIIncompatible:           ExitCompatibility,
	CodeInternalInvariant:         ExitInternal,
}

// Failure is the shared daemon and CLI JSON error payload. Message, Field, and
// Resource must be redacted diagnostics; providers must not place raw command
// output or secret values in them.
type Failure struct {
	SchemaVersion string    `json:"schemaVersion"`
	Code          ErrorCode `json:"code"`
	Message       string    `json:"message"`
	Field         string    `json:"field,omitempty"`
	Resource      string    `json:"resource,omitempty"`
	Retryable     bool      `json:"retryable"`
}

func (f Failure) Error() string { return f.Message }

// ExitCode maps a symbolic error to its process status. Unknown codes fail
// closed as internal invariant violations.
func (f Failure) ExitCode() int { return ExitCodeFor(f.Code) }

func (c ErrorCode) Valid() bool {
	_, ok := exitCodes[c]
	return ok
}

func ExitCodeFor(code ErrorCode) int {
	if value, ok := exitCodes[code]; ok {
		return value
	}
	return ExitInternal
}

// ErrorCodes returns a sorted copy for compatibility checks and documentation.
func ErrorCodes() []ErrorCode {
	codes := make([]ErrorCode, 0, len(exitCodes))
	for code := range exitCodes {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	return codes
}

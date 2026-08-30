// Package doctor produces deterministic read-only prerequisite reports.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

const SchemaVersion = "abox.doctor/v1alpha1"

type Mode string

const (
	ModeMachine    Mode = "machine"
	ModeBox        Mode = "box"
	ModeFilesystem Mode = "filesystem"
)

type Evidence string

const (
	EvidenceTested      Evidence = "tested"
	EvidenceWarning     Evidence = "warning"
	EvidenceUnsupported Evidence = "unsupported"
	EvidenceUnknown     Evidence = "unknown"
)

type Status string

const (
	StatusPass     Status = "pass"
	StatusFail     Status = "fail"
	StatusAdvisory Status = "advisory"
)

// Check is one bounded diagnostic. Result is a stable symbolic identity; human
// callers may display the other fields, but automation branches on Result.
type Check struct {
	Result      string   `json:"result"`
	Evidence    Evidence `json:"evidence"`
	Status      Status   `json:"status"`
	Observed    string   `json:"observed"`
	Required    string   `json:"required"`
	Remediation string   `json:"remediation,omitempty"`
}

type Report struct {
	SchemaVersion string  `json:"schemaVersion"`
	Mode          Mode    `json:"mode"`
	TaskKey       string  `json:"taskKey,omitempty"`
	Checks        []Check `json:"checks"`
}

type Request struct {
	Mode    Mode
	TaskKey string
}

// Observer has observation capability only. It cannot provision or repair a
// resource through this contract.
type Observer interface {
	Observe(context.Context, Request) ([]Check, error)
}

// Inspect validates and canonicalizes observations into a stable report.
func Inspect(ctx context.Context, request Request, observer Observer) (Report, error) {
	if err := validateRequest(request); err != nil {
		return Report{}, err
	}
	if observer == nil {
		return Report{}, errors.New("doctor observer is nil")
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	checks, err := observer.Observe(ctx, request)
	if err != nil {
		return Report{}, err
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if len(checks) == 0 {
		return Report{}, errors.New("doctor observation is empty")
	}
	checks = append([]Check(nil), checks...)
	slices.SortFunc(checks, func(left, right Check) int { return strings.Compare(left.Result, right.Result) })
	for index, check := range checks {
		if err := validateCheck(check); err != nil {
			return Report{}, fmt.Errorf("doctor check %d: %w", index, err)
		}
		if index > 0 && check.Result == checks[index-1].Result {
			return Report{}, errors.New("doctor observation contains a duplicate result")
		}
	}
	return Report{SchemaVersion: SchemaVersion, Mode: request.Mode, TaskKey: request.TaskKey, Checks: checks}, nil
}

// Failed reports whether a tested or unsupported prerequisite failed. Warnings
// and unknown evidence remain useful diagnostics and do not make the command
// itself fail.
func (r Report) Failed() bool {
	for _, check := range r.Checks {
		if check.Status == StatusFail {
			return true
		}
	}
	return false
}

func validateRequest(request Request) error {
	switch request.Mode {
	case ModeMachine:
		if request.TaskKey != "" {
			return errors.New("machine doctor cannot identify a task")
		}
	case ModeBox, ModeFilesystem:
		if !validText(request.TaskKey) {
			return errors.New("box and filesystem doctor require a task key")
		}
	default:
		return errors.New("doctor mode is invalid")
	}
	return nil
}

func validateCheck(check Check) error {
	if !validResult(check.Result) || !validText(check.Observed) || !validText(check.Required) {
		return errors.New("result, observed, and required facts must be bounded text")
	}
	switch check.Evidence {
	case EvidenceTested:
		if check.Status != StatusPass && check.Status != StatusFail {
			return errors.New("tested evidence must pass or fail")
		}
	case EvidenceWarning, EvidenceUnknown:
		if check.Status != StatusAdvisory {
			return errors.New("warning and unknown evidence must be advisory")
		}
	case EvidenceUnsupported:
		if check.Status != StatusFail && check.Status != StatusAdvisory {
			return errors.New("unsupported evidence must fail or be advisory")
		}
	default:
		return errors.New("evidence class is invalid")
	}
	if check.Status != StatusPass && !validText(check.Remediation) {
		return errors.New("non-passing evidence requires remediation")
	}
	if check.Status == StatusPass && check.Remediation != "" {
		return errors.New("passing evidence cannot include remediation")
	}
	return nil
}

func validResult(value string) bool {
	if !validText(value) || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '.' && character != '-' {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	return value != "" && len(value) <= 1024 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}

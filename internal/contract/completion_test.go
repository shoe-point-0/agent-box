package contract_test

import (
	"encoding/json"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
)

func TestNewOperationResultPreservesTerminalPayload(t *testing.T) {
	const invariant = `{"apiVersion":"abox.api/v1alpha1","state":"failed","errorCode":"ABOX_INTERNAL_INVARIANT"}`
	for _, test := range []struct {
		name    string
		failure *contract.Failure
		want    string
	}{
		{"success", nil, `{"apiVersion":"abox.api/v1alpha1","state":"succeeded"}`},
		{"known code", &contract.Failure{Code: contract.CodeCleanupRefused, Message: "runner diagnostic", Retryable: true},
			`{"apiVersion":"abox.api/v1alpha1","state":"failed","errorCode":"ABOX_CLEANUP_REFUSED"}`},
		{"unknown code", &contract.Failure{Code: "ABOX_UNKNOWN"}, invariant},
		{"missing code", &contract.Failure{}, invariant},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := contract.NewOperationResult(test.failure)
			if err := result.Validate(); err != nil {
				t.Fatalf("constructed result is invalid: %v", err)
			}
			got, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != test.want {
				t.Fatalf("terminal payload = %s, want %s", got, test.want)
			}
		})
	}
}

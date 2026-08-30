// Package ownership verifies recorded and live facts before destructive adapters
// are allowed to identify a target. It decides authorization, but never mutates
// an external resource.
package ownership

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotOwned marks a failed ownership proof.
var ErrNotOwned = errors.New("ownership proof refused")

// BoxIDFact is the required independently observed Agent Box ownership fact
// on labeled or metadata-bearing external resources.
const BoxIDFact = "dev.abox.box-id"

// ComposeBoxIDFact is the Box identity label required by the versioned
// Compose contract. Lima metadata uses BoxIDFact; Docker resources use this
// label because it is the name projected by the generated Compose override.
const ComposeBoxIDFact = "dev.abox.box"

// ProofError identifies the resource class and fact that failed without
// including potentially sensitive observed values.
type ProofError struct {
	Resource string
	Fact     string
}

func (e *ProofError) Error() string {
	return fmt.Sprintf("prove %s ownership: %s: %v", e.Resource, e.Fact, ErrNotOwned)
}

func (e *ProofError) Unwrap() error { return ErrNotOwned }

func refuse(resource, fact string) error {
	return &ProofError{Resource: resource, Fact: fact}
}

// LimaInstanceRecord is the identity persisted when Agent Box creates a Lima
// instance. Metadata must contain at least one independently observable value,
// normally the Box identity.
type LimaInstanceRecord struct {
	Name     string
	Metadata map[string]string
}

// LimaInstanceObservation is identity read from the live Lima instance.
type LimaInstanceObservation struct {
	Name     string
	Metadata map[string]string
}

// LimaInstanceProof authorizes a Lima mutation. Its fields are deliberately
// private so only this package can create a non-zero proof.
type LimaInstanceProof struct{ name string }

// Name returns the verified adapter target.
func (p LimaInstanceProof) Name() string { return p.name }

// ProveLimaInstance requires a recorded name and matching embedded live
// metadata. A matching name or prefix alone is insufficient.
func ProveLimaInstance(recorded LimaInstanceRecord, live LimaInstanceObservation) (LimaInstanceProof, error) {
	if !validOpaque(recorded.Name) || recorded.Name != live.Name {
		return LimaInstanceProof{}, refuse("lima instance", "recorded and live names differ")
	}
	if err := verifyBoxFacts("lima instance", "embedded metadata", BoxIDFact, recorded.Metadata, live.Metadata); err != nil {
		return LimaInstanceProof{}, err
	}
	return LimaInstanceProof{name: recorded.Name}, nil
}

// ComposeResourceRecord is the persisted identity of a Compose-managed
// container, network, image, or other non-volume resource.
type ComposeResourceRecord struct {
	Kind   string
	ID     string
	Labels map[string]string
}

// ComposeResourceObservation is identity observed from the live runtime.
type ComposeResourceObservation struct {
	Kind   string
	ID     string
	Labels map[string]string
}

// ComposeResourceProof authorizes mutation of one verified runtime object.
type ComposeResourceProof struct {
	kind string
	id   string
}

func (p ComposeResourceProof) Kind() string { return p.kind }
func (p ComposeResourceProof) ID() string   { return p.id }

// ProveComposeResource requires the recorded opaque runtime ID and every
// recorded ownership label to match live observation.
func ProveComposeResource(recorded ComposeResourceRecord, live ComposeResourceObservation) (ComposeResourceProof, error) {
	if !validOpaque(recorded.Kind) || recorded.Kind != live.Kind {
		return ComposeResourceProof{}, refuse("compose resource", "resource kinds differ")
	}
	if !validOpaque(recorded.ID) || recorded.ID != live.ID {
		return ComposeResourceProof{}, refuse("compose resource", "opaque IDs differ")
	}
	if err := verifyBoxFacts("compose resource", "ownership labels", ComposeBoxIDFact, recorded.Labels, live.Labels); err != nil {
		return ComposeResourceProof{}, err
	}
	return ComposeResourceProof{kind: recorded.Kind, id: recorded.ID}, nil
}

// VolumeRecord is the persisted identity of a Box-owned volume.
type VolumeRecord struct {
	ID     string
	Labels map[string]string
}

// VolumeObservation is identity observed from the live runtime.
type VolumeObservation struct {
	ID     string
	Labels map[string]string
}

// VolumeProof authorizes mutation of one verified volume.
type VolumeProof struct{ id string }

func (p VolumeProof) ID() string { return p.id }

// ProveVolume requires both the recorded opaque ID and independently observed
// ownership labels. A generated volume name is not accepted as proof.
func ProveVolume(recorded VolumeRecord, live VolumeObservation) (VolumeProof, error) {
	if !validOpaque(recorded.ID) || recorded.ID != live.ID {
		return VolumeProof{}, refuse("volume", "opaque IDs differ")
	}
	if err := verifyBoxFacts("volume", "ownership labels", ComposeBoxIDFact, recorded.Labels, live.Labels); err != nil {
		return VolumeProof{}, err
	}
	return VolumeProof{id: recorded.ID}, nil
}

// ListenerRecord is the persisted identity of a listener and the daemon lease
// that owns it.
type ListenerRecord struct {
	Network    string
	Address    string
	ListenerID string
	LeaseID    string
}

// ListenerObservation is identity observed from the active listener registry.
type ListenerObservation struct {
	Network     string
	Address     string
	ListenerID  string
	LeaseID     string
	LeaseActive bool
}

// ListenerProof authorizes closing or replacing one verified listener.
type ListenerProof struct {
	network    string
	address    string
	listenerID string
	leaseID    string
}

func (p ListenerProof) Network() string    { return p.network }
func (p ListenerProof) Address() string    { return p.address }
func (p ListenerProof) ListenerID() string { return p.listenerID }
func (p ListenerProof) LeaseID() string    { return p.leaseID }

// ProveListener requires bound-listener identity and a matching active daemon
// lease. An address alone never authorizes a destructive operation.
func ProveListener(recorded ListenerRecord, live ListenerObservation) (ListenerProof, error) {
	if !validOpaque(recorded.Network) || !validOpaque(recorded.Address) ||
		!validOpaque(recorded.ListenerID) || !validOpaque(recorded.LeaseID) {
		return ListenerProof{}, refuse("listener", "recorded identity is incomplete")
	}
	if recorded.Network != live.Network || recorded.Address != live.Address || recorded.ListenerID != live.ListenerID {
		return ListenerProof{}, refuse("listener", "bound identity differs")
	}
	if !live.LeaseActive || recorded.LeaseID != live.LeaseID {
		return ListenerProof{}, refuse("listener", "daemon lease is inactive or differs")
	}
	return ListenerProof{
		network: recorded.Network, address: recorded.Address,
		listenerID: recorded.ListenerID, leaseID: recorded.LeaseID,
	}, nil
}

func verifyBoxFacts(resource, fact, boxIDFact string, recorded, live map[string]string) error {
	if !validOpaque(recorded[boxIDFact]) {
		return refuse(resource, fact+" lack the Agent Box identity")
	}
	for key, expected := range recorded {
		if !validOpaque(key) || !validOpaque(expected) {
			return refuse(resource, fact+" contain an empty or invalid fact")
		}
		if observed, ok := live[key]; !ok || observed != expected {
			return refuse(resource, fact+" differ")
		}
	}
	return nil
}

func validOpaque(value string) bool {
	return value != "" && !strings.ContainsRune(value, '\x00')
}

package domain

import "time"

// ArtifactLifecycle tracks the storage lifecycle of an Artifact.
type ArtifactLifecycle string

const (
	ArtifactCreated   ArtifactLifecycle = "CREATED"
	ArtifactUploading ArtifactLifecycle = "UPLOADING"
	ArtifactAvailable ArtifactLifecycle = "AVAILABLE"
	ArtifactExpired   ArtifactLifecycle = "EXPIRED"
	ArtifactDeleted   ArtifactLifecycle = "DELETED"
)

// Artifact is a durable product (log, dump, firmware, report, screenshot,
// binary). Metadata lives in a metadata store; the large object lives in an
// object store addressed by URI.
//
// Content is the in-memory payload carried from a Plugin to the Artifact
// Service; it is never part of the wire contract (see api/proto Artifact).
type Artifact struct {
	ID          ArtifactID
	Type        string
	URI         string
	Checksum    string
	SizeBytes   int64
	ContentType string
	OperationID OperationID
	Lifecycle   ArtifactLifecycle
	CreatedAt   time.Time
	Content     []byte
}

// NewArtifact builds an Artifact in CREATED state.
func NewArtifact(typ string) *Artifact {
	return &Artifact{
		ID:        NewArtifactID(),
		Type:      typ,
		Lifecycle: ArtifactCreated,
		CreatedAt: time.Now(),
	}
}

// Evidence is proof that supports a Result. It is either a structured value
// or a reference to an Artifact.
type Evidence struct {
	ID          EvidenceID
	Name        string
	Value       string
	ArtifactRef ArtifactID
	OperationID OperationID
}

// NewEvidence builds a structured Evidence value.
func NewEvidence(name, value string) *Evidence {
	return &Evidence{
		ID:    NewEvidenceID(),
		Name:  name,
		Value: value,
	}
}

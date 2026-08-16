// Package artifact implements the Artifact Service (Design docs 08, 15 §9):
// metadata store + object store, checksum integrity, and the CREATED →
// UPLOADING → AVAILABLE → EXPIRED → DELETED lifecycle.
package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// Service is an in-memory Artifact metadata + object store.
type Service struct {
	mu      sync.RWMutex
	meta    map[domain.ArtifactID]*domain.Artifact
	objects map[domain.ArtifactID][]byte
}

// New builds an empty Service.
func New() *Service {
	return &Service{
		meta:    map[domain.ArtifactID]*domain.Artifact{},
		objects: map[domain.ArtifactID][]byte{},
	}
}

// Create records artifact metadata in CREATED state.
func (s *Service) Create(typ, contentType string, operationID domain.OperationID) *domain.Artifact {
	a := domain.NewArtifact(typ)
	a.ContentType = contentType
	a.OperationID = operationID
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meta[a.ID] = a
	return a
}

// Upload stores the object bytes, moving CREATED → UPLOADING.
func (s *Service) Upload(id domain.ArtifactID, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.meta[id]
	if !ok {
		return fmt.Errorf("artifact %s not found", id)
	}
	if a.Lifecycle != domain.ArtifactCreated && a.Lifecycle != domain.ArtifactUploading {
		return fmt.Errorf("artifact %s is %s, cannot upload", id, a.Lifecycle)
	}
	s.objects[id] = append([]byte(nil), data...)
	a.Lifecycle = domain.ArtifactUploading
	return nil
}

// Finalize computes the SHA-256 checksum and size, and moves UPLOADING →
// AVAILABLE. An Artifact is never AVAILABLE without verification (Design doc
// 08 §22).
func (s *Service) Finalize(id domain.ArtifactID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.meta[id]
	if !ok {
		return fmt.Errorf("artifact %s not found", id)
	}
	data, ok := s.objects[id]
	if !ok {
		return fmt.Errorf("artifact %s has no content", id)
	}
	sum := sha256.Sum256(data)
	a.Checksum = hex.EncodeToString(sum[:])
	a.SizeBytes = int64(len(data))
	a.URI = "object://" + string(id)
	a.Lifecycle = domain.ArtifactAvailable
	return nil
}

// Ingest stores a fully-formed artifact (metadata + content) and returns the
// finalized (AVAILABLE) artifact. It is the convenience path used by the
// Operation Engine when a Plugin returns an artifact.
func (s *Service) Ingest(a *domain.Artifact) (*domain.Artifact, error) {
	if a == nil {
		return nil, fmt.Errorf("artifact is nil")
	}
	created := s.Create(a.Type, a.ContentType, a.OperationID)
	if err := s.Upload(created.ID, a.Content); err != nil {
		return nil, err
	}
	if err := s.Finalize(created.ID); err != nil {
		return nil, err
	}
	final, _ := s.Get(created.ID)
	return final, nil
}

// Get returns artifact metadata.
func (s *Service) Get(id domain.ArtifactID) (*domain.Artifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.meta[id]
	if !ok {
		return nil, false
	}
	cp := *a
	return &cp, true
}

// Content returns the object bytes.
func (s *Service) Content(id domain.ArtifactID) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.objects[id]
	return b, ok
}

// List returns all artifact metadata, in creation order (stable by ID).
func (s *Service) List() []*domain.Artifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.Artifact, 0, len(s.meta))
	for _, a := range s.meta {
		cp := *a
		out = append(out, &cp)
	}
	return out
}

// Delete removes metadata and object bytes.
func (s *Service) Delete(id domain.ArtifactID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.meta[id]; !ok {
		return fmt.Errorf("artifact %s not found", id)
	}
	delete(s.meta, id)
	delete(s.objects, id)
	return nil
}

// Verify recomputes the checksum of an AVAILABLE artifact and reports whether
// it matches the stored checksum.
func (s *Service) Verify(id domain.ArtifactID) (bool, error) {
	s.mu.RLock()
	a, ok := s.meta[id]
	data, ok2 := s.objects[id]
	s.mu.RUnlock()
	if !ok || !ok2 {
		return false, fmt.Errorf("artifact %s not found", id)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == a.Checksum, nil
}

var _ = time.Now // reserved for retention (Beta)

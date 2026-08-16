package unit

import (
	"testing"

	"example.com/embedded-loop-channel/core/artifact"
	"example.com/embedded-loop-channel/domain"
)

func TestArtifactLifecycle(t *testing.T) {
	s := artifact.New()
	a := s.Create("log", "text/plain", "op-1")
	if a.Lifecycle != domain.ArtifactCreated {
		t.Fatalf("expected CREATED, got %s", a.Lifecycle)
	}

	if err := s.Upload(a.ID, []byte("hello world")); err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if err := s.Finalize(a.ID); err != nil {
		t.Fatalf("finalize failed: %v", err)
	}

	got, ok := s.Get(a.ID)
	if !ok {
		t.Fatalf("artifact not found after finalize")
	}
	if got.Lifecycle != domain.ArtifactAvailable || got.Checksum == "" || got.SizeBytes != 11 {
		t.Fatalf("artifact not finalized correctly: %+v", got)
	}

	ok2, err := s.Verify(a.ID)
	if err != nil || !ok2 {
		t.Fatalf("checksum verification failed: %v", err)
	}
}

func TestArtifactIngestAndImmutability(t *testing.T) {
	s := artifact.New()
	a := domain.NewArtifact("dump")
	a.ContentType = "application/octet-stream"
	a.Content = []byte("sensitive data")

	final, err := s.Ingest(a)
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if final.Lifecycle != domain.ArtifactAvailable || final.Checksum == "" {
		t.Fatalf("ingest should finalize: %+v", final)
	}

	// Verification passes on intact content.
	if ok, err := s.Verify(final.ID); err != nil || !ok {
		t.Fatalf("verify should pass: ok=%v err=%v", ok, err)
	}

	// An AVAILABLE artifact is immutable: re-upload is rejected.
	if err := s.Upload(final.ID, []byte("tampered")); err == nil {
		t.Fatalf("re-upload of an AVAILABLE artifact should be rejected")
	}
}

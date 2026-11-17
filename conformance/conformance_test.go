package conformance

import (
	"testing"
)

func TestConformanceVectors(t *testing.T) {
	vectors, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) < 10 {
		t.Fatalf("expected at least 10 vectors, got %d", len(vectors))
	}
	for _, v := range vectors {
		v := v
		t.Run(v.ID, func(t *testing.T) {
			RunVector(t, v)
		})
	}
}

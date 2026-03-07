package main

import (
	"testing"
)

func TestFindAvailablePort(t *testing.T) {
	port, err := findAvailablePort()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Errorf("expected valid port, got: %d", port)
	}
}

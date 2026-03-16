package resolver

import (
	"os"
	"path/filepath"
	"testing"
)

// writeAgents writes content to a temp AGENTS.md and returns its path.
func writeAgents(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing AGENTS.md: %v", err)
	}
	return path
}

func TestParseDougMeta_HappyPath(t *testing.T) {
	path := writeAgents(t, `# Agent Onboarding

Some content here.

<!-- DOUG-SPECIFIC-INSTRUCTIONS:START -->
DOUG_PROJECT_ID: my-project-abc123
DOUG_PROJECT_NAME: My Project

## Doug-Specific Instructions

Some other content.
<!-- DOUG-SPECIFIC-INSTRUCTIONS:END -->
`)
	got := ParseDougMeta(path)
	if got.ProjectID != "my-project-abc123" {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, "my-project-abc123")
	}
	if got.ProjectName != "My Project" {
		t.Errorf("ProjectName = %q, want %q", got.ProjectName, "My Project")
	}
}

func TestParseDougMeta_IDOnly(t *testing.T) {
	path := writeAgents(t, `<!-- DOUG-SPECIFIC-INSTRUCTIONS:START -->
DOUG_PROJECT_ID: only-id-xyz
<!-- DOUG-SPECIFIC-INSTRUCTIONS:END -->
`)
	got := ParseDougMeta(path)
	if got.ProjectID != "only-id-xyz" {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, "only-id-xyz")
	}
	if got.ProjectName != "" {
		t.Errorf("ProjectName = %q, want empty", got.ProjectName)
	}
}

func TestParseDougMeta_AbsentFile(t *testing.T) {
	got := ParseDougMeta("/nonexistent/path/AGENTS.md")
	if got.ProjectID != "" || got.ProjectName != "" {
		t.Errorf("expected empty DougMeta for absent file, got %+v", got)
	}
}

func TestParseDougMeta_MissingBlock(t *testing.T) {
	path := writeAgents(t, `# Agent Onboarding

DOUG_PROJECT_ID: should-not-be-parsed
DOUG_PROJECT_NAME: Should Not Be Parsed
`)
	got := ParseDougMeta(path)
	if got.ProjectID != "" || got.ProjectName != "" {
		t.Errorf("expected empty DougMeta when managed block is missing, got %+v", got)
	}
}

func TestParseDougMeta_KeysOutsideBlockIgnored(t *testing.T) {
	// Keys before the start marker must be ignored.
	path := writeAgents(t, `DOUG_PROJECT_ID: before-block

<!-- DOUG-SPECIFIC-INSTRUCTIONS:START -->
DOUG_PROJECT_ID: inside-block
<!-- DOUG-SPECIFIC-INSTRUCTIONS:END -->

DOUG_PROJECT_ID: after-block
`)
	got := ParseDougMeta(path)
	if got.ProjectID != "inside-block" {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, "inside-block")
	}
}

func TestParseDougMeta_DuplicateIDUsesFirst(t *testing.T) {
	path := writeAgents(t, `<!-- DOUG-SPECIFIC-INSTRUCTIONS:START -->
DOUG_PROJECT_ID: first-id
DOUG_PROJECT_ID: second-id
<!-- DOUG-SPECIFIC-INSTRUCTIONS:END -->
`)
	got := ParseDougMeta(path)
	if got.ProjectID != "first-id" {
		t.Errorf("ProjectID = %q, want %q (first value)", got.ProjectID, "first-id")
	}
}

func TestParseDougMeta_DuplicateNameUsesFirst(t *testing.T) {
	path := writeAgents(t, `<!-- DOUG-SPECIFIC-INSTRUCTIONS:START -->
DOUG_PROJECT_NAME: First Name
DOUG_PROJECT_NAME: Second Name
<!-- DOUG-SPECIFIC-INSTRUCTIONS:END -->
`)
	got := ParseDougMeta(path)
	if got.ProjectName != "First Name" {
		t.Errorf("ProjectName = %q, want %q (first value)", got.ProjectName, "First Name")
	}
}

func TestParseDougMeta_EmptyFile(t *testing.T) {
	path := writeAgents(t, "")
	got := ParseDougMeta(path)
	if got.ProjectID != "" || got.ProjectName != "" {
		t.Errorf("expected empty DougMeta for empty file, got %+v", got)
	}
}

func TestParseDougMeta_ValueWithLeadingSpaces(t *testing.T) {
	path := writeAgents(t, `<!-- DOUG-SPECIFIC-INSTRUCTIONS:START -->
DOUG_PROJECT_ID:   spaced-value
DOUG_PROJECT_NAME:   Spaced Name
<!-- DOUG-SPECIFIC-INSTRUCTIONS:END -->
`)
	got := ParseDougMeta(path)
	if got.ProjectID != "spaced-value" {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, "spaced-value")
	}
	if got.ProjectName != "Spaced Name" {
		t.Errorf("ProjectName = %q, want %q", got.ProjectName, "Spaced Name")
	}
}

// TestParseDougMeta_FeedsResolver verifies that ParseDougMeta output correctly
// drives resolver priority when passed through resolver.Input.
func TestParseDougMeta_FeedsResolver(t *testing.T) {
	path := writeAgents(t, `<!-- DOUG-SPECIFIC-INSTRUCTIONS:START -->
DOUG_PROJECT_ID: doug-stats-ca65d3
DOUG_PROJECT_NAME: Doug Stats
<!-- DOUG-SPECIFIC-INSTRUCTIONS:END -->
`)
	meta := ParseDougMeta(path)
	res := Resolve(Input{
		DougProjectID:   meta.ProjectID,
		DougProjectName: meta.ProjectName,
		GitRemoteURL:    "https://github.com/owner/other-repo",
		RawPath:         "/home/user/other-path",
	})

	if res.CanonicalProjectID != "doug-stats-ca65d3" {
		t.Errorf("CanonicalProjectID = %q, want %q", res.CanonicalProjectID, "doug-stats-ca65d3")
	}
	if res.DisplayProjectName != "Doug Stats" {
		t.Errorf("DisplayProjectName = %q, want %q", res.DisplayProjectName, "Doug Stats")
	}
}

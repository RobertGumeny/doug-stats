package resolver

import (
	"testing"

	"github.com/robertgumeny/doug-stats/provider"
)

// --- Resolve: four primary resolution paths ---

func TestResolve_DougProjectID(t *testing.T) {
	in := Input{
		DougProjectID:   "doug-stats-ca65d3",
		DougProjectName: "Doug Stats",
		GitRemoteURL:    "https://github.com/owner/doug-stats",
		RawPath:         "/home/user/doug-stats",
	}
	got := Resolve(in)

	if got.CanonicalProjectID != "doug-stats-ca65d3" {
		t.Errorf("CanonicalProjectID = %q, want %q", got.CanonicalProjectID, "doug-stats-ca65d3")
	}
	if got.CanonicalProjectSource != provider.SourceDoug {
		t.Errorf("CanonicalProjectSource = %q, want %q", got.CanonicalProjectSource, provider.SourceDoug)
	}
	if got.DisplayProjectName != "Doug Stats" {
		t.Errorf("DisplayProjectName = %q, want %q", got.DisplayProjectName, "Doug Stats")
	}
}

func TestResolve_DougProjectID_NoName(t *testing.T) {
	// When DougProjectName is absent, DisplayProjectName falls back to DougProjectID.
	in := Input{DougProjectID: "myproject-abc123"}
	got := Resolve(in)

	if got.CanonicalProjectID != "myproject-abc123" {
		t.Errorf("CanonicalProjectID = %q, want %q", got.CanonicalProjectID, "myproject-abc123")
	}
	if got.CanonicalProjectSource != provider.SourceDoug {
		t.Errorf("CanonicalProjectSource = %q, want %q", got.CanonicalProjectSource, provider.SourceDoug)
	}
	if got.DisplayProjectName != "myproject-abc123" {
		t.Errorf("DisplayProjectName = %q, want %q", got.DisplayProjectName, "myproject-abc123")
	}
}

func TestResolve_GitRemote_HTTPS(t *testing.T) {
	in := Input{
		GitRemoteURL: "https://github.com/owner/my-repo.git",
		RawPath:      "/home/user/my-repo",
	}
	got := Resolve(in)

	if got.CanonicalProjectID != "my-repo" {
		t.Errorf("CanonicalProjectID = %q, want %q", got.CanonicalProjectID, "my-repo")
	}
	if got.CanonicalProjectSource != provider.SourceGitRemote {
		t.Errorf("CanonicalProjectSource = %q, want %q", got.CanonicalProjectSource, provider.SourceGitRemote)
	}
	if got.DisplayProjectName != "my-repo" {
		t.Errorf("DisplayProjectName = %q, want %q", got.DisplayProjectName, "my-repo")
	}
}

func TestResolve_GitRemote_SCP(t *testing.T) {
	in := Input{
		GitRemoteURL: "git@github.com:owner/my-repo.git",
	}
	got := Resolve(in)

	if got.CanonicalProjectID != "my-repo" {
		t.Errorf("CanonicalProjectID = %q, want %q", got.CanonicalProjectID, "my-repo")
	}
	if got.CanonicalProjectSource != provider.SourceGitRemote {
		t.Errorf("CanonicalProjectSource = %q, want %q", got.CanonicalProjectSource, provider.SourceGitRemote)
	}
}

func TestResolve_NormalizedPath(t *testing.T) {
	in := Input{
		RawPath: "/home/User/MyProject",
	}
	got := Resolve(in)

	if got.CanonicalProjectID != "/home/user/myproject" {
		t.Errorf("CanonicalProjectID = %q, want %q", got.CanonicalProjectID, "/home/user/myproject")
	}
	if got.CanonicalProjectSource != provider.SourceNormalizedPath {
		t.Errorf("CanonicalProjectSource = %q, want %q", got.CanonicalProjectSource, provider.SourceNormalizedPath)
	}
	if got.DisplayProjectName != "MyProject" {
		t.Errorf("DisplayProjectName = %q, want %q", got.DisplayProjectName, "MyProject")
	}
}

func TestResolve_BasenameFallback(t *testing.T) {
	in := Input{
		RawPath: "my-project",
	}
	got := Resolve(in)

	if got.CanonicalProjectID != "my-project" {
		t.Errorf("CanonicalProjectID = %q, want %q", got.CanonicalProjectID, "my-project")
	}
	if got.CanonicalProjectSource != provider.SourceBasenameFallback {
		t.Errorf("CanonicalProjectSource = %q, want %q", got.CanonicalProjectSource, provider.SourceBasenameFallback)
	}
	if got.DisplayProjectName != "my-project" {
		t.Errorf("DisplayProjectName = %q, want %q", got.DisplayProjectName, "my-project")
	}
}

// --- Priority ordering with conflicting metadata ---

func TestResolve_DougWinsOverGitAndPath(t *testing.T) {
	// Doug ID takes precedence when all three sources are present.
	in := Input{
		DougProjectID:   "proj-xyz",
		DougProjectName: "My Project",
		GitRemoteURL:    "https://github.com/owner/other-repo",
		RawPath:         "/home/user/other-path",
	}
	got := Resolve(in)

	if got.CanonicalProjectSource != provider.SourceDoug {
		t.Errorf("expected SourceDoug, got %q", got.CanonicalProjectSource)
	}
	if got.CanonicalProjectID != "proj-xyz" {
		t.Errorf("CanonicalProjectID = %q, want %q", got.CanonicalProjectID, "proj-xyz")
	}
}

func TestResolve_GitRemoteWinsOverPath(t *testing.T) {
	// Git remote wins over path when Doug ID is absent.
	in := Input{
		GitRemoteURL: "https://github.com/owner/remote-repo",
		RawPath:      "/home/user/local-path",
	}
	got := Resolve(in)

	if got.CanonicalProjectSource != provider.SourceGitRemote {
		t.Errorf("expected SourceGitRemote, got %q", got.CanonicalProjectSource)
	}
	if got.CanonicalProjectID != "remote-repo" {
		t.Errorf("CanonicalProjectID = %q, want %q", got.CanonicalProjectID, "remote-repo")
	}
}

func TestResolve_NormalizedPathWinsOverBasename(t *testing.T) {
	// Absolute path wins over basename when git remote is absent.
	in := Input{RawPath: "/home/user/my-repo"}
	got := Resolve(in)

	if got.CanonicalProjectSource != provider.SourceNormalizedPath {
		t.Errorf("expected SourceNormalizedPath, got %q", got.CanonicalProjectSource)
	}
}

// --- Missing metadata cases ---

func TestResolve_AllEmpty_Fallback(t *testing.T) {
	// Completely empty input falls back to basename of empty string.
	got := Resolve(Input{})

	if got.CanonicalProjectSource != provider.SourceBasenameFallback {
		t.Errorf("expected SourceBasenameFallback, got %q", got.CanonicalProjectSource)
	}
}

func TestResolve_EmptyGitRemote_UsesPath(t *testing.T) {
	in := Input{RawPath: "/srv/projects/api"}
	got := Resolve(in)

	if got.CanonicalProjectSource != provider.SourceNormalizedPath {
		t.Errorf("expected SourceNormalizedPath, got %q", got.CanonicalProjectSource)
	}
	if got.CanonicalProjectID != "/srv/projects/api" {
		t.Errorf("CanonicalProjectID = %q, want %q", got.CanonicalProjectID, "/srv/projects/api")
	}
}

// --- repoSlugFromRemote ---

func TestRepoSlugFromRemote(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://github.com/owner/my-repo.git", "my-repo"},
		{"https://github.com/owner/my-repo", "my-repo"},
		{"git@github.com:owner/my-repo.git", "my-repo"},
		{"git@github.com:owner/my-repo", "my-repo"},
		{"ssh://git@github.com/owner/my-repo.git", "my-repo"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		got := repoSlugFromRemote(tc.input)
		if got != tc.want {
			t.Errorf("repoSlugFromRemote(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- Cross-provider same-project merge ---

func TestResolve_CrossProvider_SameRepo(t *testing.T) {
	// A Claude session and a Gemini session for the same repo (identified by
	// git remote) should produce identical CanonicalProjectIDs.
	claudeSession := Input{
		GitRemoteURL: "https://github.com/acme/backend.git",
		RawPath:      "/home/alice/backend",
	}
	geminiSession := Input{
		GitRemoteURL: "git@github.com:acme/backend.git",
		RawPath:      "/home/bob/projects/backend",
	}

	rClaude := Resolve(claudeSession)
	rGemini := Resolve(geminiSession)

	if rClaude.CanonicalProjectID != rGemini.CanonicalProjectID {
		t.Errorf("cross-provider mismatch: claude=%q gemini=%q",
			rClaude.CanonicalProjectID, rGemini.CanonicalProjectID)
	}
	if rClaude.CanonicalProjectSource != provider.SourceGitRemote {
		t.Errorf("expected SourceGitRemote for claude session, got %q", rClaude.CanonicalProjectSource)
	}
	if rGemini.CanonicalProjectSource != provider.SourceGitRemote {
		t.Errorf("expected SourceGitRemote for gemini session, got %q", rGemini.CanonicalProjectSource)
	}
}

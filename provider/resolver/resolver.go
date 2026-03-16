// Package resolver resolves raw provider session metadata into a canonical
// project identity using a defined priority order.
//
// Resolution priority:
//  1. Doug project ID — read from AGENTS.md (DOUG_PROJECT_ID field)
//  2. Git remote slug — basename derived from the git remote URL
//  3. Normalized filesystem path — lowercase-cleaned absolute path
//  4. Basename fallback — filepath.Base of whatever raw path was provided
package resolver

import (
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/robertgumeny/doug-stats/provider"
)

// Input holds the raw metadata available at resolution time.
// All fields are optional; missing fields cause the resolver to fall through
// to the next priority level.
type Input struct {
	// DougProjectID is the DOUG_PROJECT_ID value from AGENTS.md, if present.
	DougProjectID string
	// DougProjectName is the DOUG_PROJECT_NAME value from AGENTS.md, if present.
	DougProjectName string
	// GitRemoteURL is the raw git remote URL (e.g. from a provider's session
	// metadata), used to derive a stable repo slug.
	GitRemoteURL string
	// RawPath is the raw filesystem path provided by the provider (absolute or
	// basename-only). Used for the normalized-path and basename-fallback levels.
	RawPath string
}

// Result contains the resolved canonical project identity fields.
type Result struct {
	CanonicalProjectID     string
	CanonicalProjectSource provider.CanonicalProjectSource
	DisplayProjectName     string
}

// Resolve applies the four-level priority order and returns the canonical
// project identity for the given input.
func Resolve(in Input) Result {
	// Priority 1: Doug project ID.
	if in.DougProjectID != "" {
		name := in.DougProjectName
		if name == "" {
			name = in.DougProjectID
		}
		return Result{
			CanonicalProjectID:     in.DougProjectID,
			CanonicalProjectSource: provider.SourceDoug,
			DisplayProjectName:     name,
		}
	}

	// Priority 2: Git remote slug.
	if slug := repoSlugFromRemote(in.GitRemoteURL); slug != "" {
		return Result{
			CanonicalProjectID:     slug,
			CanonicalProjectSource: provider.SourceGitRemote,
			DisplayProjectName:     slug,
		}
	}

	// Priority 3: Normalized absolute filesystem path.
	if in.RawPath != "" {
		cleaned := filepath.Clean(in.RawPath)
		if filepath.IsAbs(cleaned) {
			normalized := strings.ToLower(cleaned)
			return Result{
				CanonicalProjectID:     normalized,
				CanonicalProjectSource: provider.SourceNormalizedPath,
				DisplayProjectName:     filepath.Base(cleaned),
			}
		}
	}

	// Priority 4: Basename fallback.
	base := filepath.Base(in.RawPath)
	if base == "." || base == "" {
		base = in.RawPath
	}
	return Result{
		CanonicalProjectID:     base,
		CanonicalProjectSource: provider.SourceBasenameFallback,
		DisplayProjectName:     base,
	}
}

// repoSlugFromRemote extracts the repository name (without .git suffix) from a
// git remote URL. Returns an empty string when the URL is empty or unparseable.
func repoSlugFromRemote(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(strings.TrimSuffix(raw, "/"), ".git")
	if raw == "" {
		return ""
	}

	// SCP-style: git@github.com:owner/repo
	if strings.Contains(raw, "@") && strings.Contains(raw, ":") && !strings.Contains(raw, "://") {
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) == 2 {
			if b := path.Base(parts[1]); b != "" && b != "." && b != "/" {
				return b
			}
		}
	}

	// URL-style: https://github.com/owner/repo or ssh://git@github.com/owner/repo
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		if b := path.Base(strings.TrimSuffix(u.Path, "/")); b != "" && b != "." && b != "/" {
			return b
		}
	}

	// Last-segment fallback for any other format.
	segs := strings.FieldsFunc(raw, func(r rune) bool { return r == '/' || r == ':' })
	if len(segs) > 0 {
		return segs[len(segs)-1]
	}
	return ""
}

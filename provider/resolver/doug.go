package resolver

import (
	"bufio"
	"log"
	"os"
	"strings"
)

const (
	managedBlockStart = "<!-- DOUG-SPECIFIC-INSTRUCTIONS:START -->"
	managedBlockEnd   = "<!-- DOUG-SPECIFIC-INSTRUCTIONS:END -->"
)

// DougMeta holds project identity metadata parsed from the doug-managed block
// in AGENTS.md.
type DougMeta struct {
	ProjectID   string
	ProjectName string
}

// ParseDougMeta reads DOUG_PROJECT_ID and DOUG_PROJECT_NAME from the
// doug-managed block in agentsFilePath. It returns an empty DougMeta when the
// file is absent, unreadable, or the managed block is not present.
//
// Duplicate keys within the managed block produce a warning; the first value
// is used.
func ParseDougMeta(agentsFilePath string) DougMeta {
	f, err := os.Open(agentsFilePath)
	if err != nil {
		return DougMeta{}
	}
	defer f.Close()

	var meta DougMeta
	inBlock := false
	seenID := false
	seenName := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == managedBlockStart {
			inBlock = true
			continue
		}
		if trimmed == managedBlockEnd {
			break
		}
		if !inBlock {
			continue
		}

		if val, ok := parseKV(line, "DOUG_PROJECT_ID"); ok {
			if seenID {
				log.Printf("warning: doug: duplicate DOUG_PROJECT_ID in %s; using first value", agentsFilePath)
			} else {
				meta.ProjectID = val
				seenID = true
			}
			continue
		}
		if val, ok := parseKV(line, "DOUG_PROJECT_NAME"); ok {
			if seenName {
				log.Printf("warning: doug: duplicate DOUG_PROJECT_NAME in %s; using first value", agentsFilePath)
			} else {
				meta.ProjectName = val
				seenName = true
			}
		}
	}

	return meta
}

// parseKV parses a line of the form "KEY: value" where KEY matches key.
// Returns (trimmed value, true) on match, ("", false) otherwise.
func parseKV(line, key string) (string, bool) {
	prefix := key + ":"
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	return strings.TrimSpace(line[len(prefix):]), true
}

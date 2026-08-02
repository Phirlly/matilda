package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateCachedSourceHandleFiles verifies that safe docs/ source handles point
// to existing local reference files from a caller-provided project root.
func ValidateCachedSourceHandleFiles(projectRoot string, handles []SourceHandle) error {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = "."
	}

	copied, err := safeSourceHandles("source handles", handles)
	if err != nil {
		return err
	}

	for _, handle := range copied {
		path := filepath.Join(root, filepath.FromSlash(handle.URI))
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("source handle %q must reference an existing cached docs file", handle.URI)
			}
			return fmt.Errorf("source handle %q cannot be accessed", handle.URI)
		}
		if info.IsDir() {
			return fmt.Errorf("source handle %q must reference a cached docs file, not a directory", handle.URI)
		}
		if isReferenceCacheNote(handle.URI) {
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("source handle %q reference note cannot be read", handle.URI)
			}
			if err := validateReferenceCacheNote(handle.URI, string(content)); err != nil {
				return err
			}
		}
	}

	return nil
}

func isReferenceCacheNote(uri string) bool {
	return strings.HasPrefix(uri, "docs/references/") &&
		strings.HasSuffix(uri, ".md") &&
		uri != "docs/references/README.md"
}

func validateReferenceCacheNote(uri string, content string) error {
	summaryHeading := referenceHeadingMatching(content, "Decision", "Design", "Facts", "Summary", "Verified")
	implementationHeading := referenceHeadingMatchingExcept(content, summaryHeading, "Boundary", "Contract", "Decision", "Implementation", "Mapping", "Requirement", "Rule", "Scope")
	required := []struct {
		name string
		ok   bool
	}{
		{name: "markdown title", ok: referenceHasTopLevelTitle(content)},
		{name: "source type or source name", ok: referenceMetadataHasValue(content, "Source type:") || referenceMetadataHasValue(content, "Source name:")},
		{name: "retrieval date", ok: referenceMetadataHasValue(content, "Retrieval date:")},
		{name: "why the reference is needed", ok: referenceLabeledSectionHasBody(content, "Why this reference is needed:")},
		{name: "source URL or retrieval handle", ok: referenceSourceSectionHasLink(content)},
		{name: "project-specific summary", ok: summaryHeading != "" && referenceSectionHasBody(content, summaryHeading)},
		{name: "implementation notes or open decisions", ok: implementationHeading != "" && referenceSectionHasBody(content, implementationHeading)},
	}
	for _, field := range required {
		if !field.ok {
			return fmt.Errorf("source handle %q reference note is missing %s", uri, field.name)
		}
	}
	return nil
}

func containsAny(content string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(content, candidate) {
			return true
		}
	}
	return false
}

func referenceHasTopLevelTitle(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# ")) != ""
		}
	}
	return false
}

func referenceMetadataHasValue(content string, label string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, label) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, label)) != ""
		}
	}
	return false
}

func referenceLabeledSectionHasBody(content string, label string) bool {
	lines := strings.Split(content, "\n")
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == label {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			return false
		}
		if trimmed != "" {
			return true
		}
	}
	return false
}

func referenceSourceSectionHasLink(content string) bool {
	for _, heading := range []string{"## Retrieval Handles", "## Official Sources"} {
		section := referenceSection(content, heading)
		if containsAny(section, "](", "http://", "https://") {
			return true
		}
	}
	return false
}

func referenceSection(content string, heading string) string {
	lines := strings.Split(content, "\n")
	var builder strings.Builder
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == heading {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if inSection {
			builder.WriteString(line)
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

func referenceSectionHasBody(content string, heading string) bool {
	section := referenceSection(content, heading)
	for _, line := range strings.Split(section, "\n") {
		if strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
}

func referenceHeadingMatching(content string, terms ...string) string {
	return referenceHeadingMatchingExcept(content, "", terms...)
}

func referenceHeadingMatchingExcept(content string, excluded string, terms ...string) string {
	for _, line := range strings.Split(content, "\n") {
		heading := strings.TrimSpace(line)
		if !strings.HasPrefix(heading, "## ") || heading == excluded {
			continue
		}
		for _, term := range terms {
			if strings.Contains(heading, term) {
				return heading
			}
		}
	}
	return ""
}

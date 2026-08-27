package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-lineage/lineage/internal/atomicfile"
)

// AiderConfigAdapter links the generated conventions file through Aider's
// project-local .aider.conf.yml without rewriting unrelated YAML.
type AiderConfigAdapter struct{}

const (
	aiderConfigFile = ".aider.conf.yml"
	aiderReadPath   = "CONVENTIONS.md"
)

func (AiderConfigAdapter) Ensure(projectRoot string) (ConfigState, error) {
	path := filepath.Join(projectRoot, aiderConfigFile)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return ConfigState{}, fmt.Errorf("read %s: %w", aiderConfigFile, err)
	}
	existed := err == nil
	text := string(data)
	lines := strings.Split(text, "\n")
	start, end, ok := aiderReadBlock(lines)
	if !ok {
		managed := []string{"read: " + aiderReadPath}
		if strings.TrimSpace(text) != "" {
			insertAt := len(lines)
			if lines[len(lines)-1] == "" {
				insertAt--
			}
			lines = append(append(append([]string(nil), lines[:insertAt]...), managed[0]), lines[insertAt:]...)
		} else {
			lines = managed
		}
		if err := atomicfile.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return ConfigState{}, fmt.Errorf("update %s: %w", aiderConfigFile, err)
		}
		return ConfigState{FileExisted: existed, CreatedFile: !existed, Managed: managed}, nil
	}

	original := append([]string(nil), lines[start:end]...)
	value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[start]), "read:"))
	if value == aiderReadPath || aiderReadListContains(lines[start+1:end], aiderReadPath) {
		return ConfigState{}, nil
	}
	var replacement []string
	if value != "" {
		replacement = []string{"read:", "  - " + value, "  - " + aiderReadPath}
	} else {
		replacement = append(append([]string(nil), original...), "  - "+aiderReadPath)
	}
	lines = append(append(append([]string(nil), lines[:start]...), replacement...), lines[end:]...)
	if err := atomicfile.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return ConfigState{}, fmt.Errorf("update %s: %w", aiderConfigFile, err)
	}
	managed := append([]string(nil), replacement...)
	return ConfigState{FileExisted: true, Original: original, Managed: managed}, nil
}

func (AiderConfigAdapter) Remove(projectRoot string, state ConfigState) error {
	if len(state.Managed) == 0 {
		return nil
	}
	path := filepath.Join(projectRoot, aiderConfigFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	start := aiderFindSequence(lines, state.Managed)
	if start < 0 {
		return nil
	}
	replacement := state.Original
	lines = append(append(append([]string(nil), lines[:start]...), replacement...), lines[start+len(state.Managed):]...)
	if state.CreatedFile && len(replacement) == 0 {
		return os.Remove(path)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func (AiderConfigAdapter) NeedsApproval(projectRoot string, state ConfigState, desired bool) (bool, error) {
	path := filepath.Join(projectRoot, aiderConfigFile)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if !desired {
		return len(state.Managed) > 0, nil
	}
	lines := strings.Split(string(data), "\n")
	start, end, ok := aiderReadBlock(lines)
	if !ok {
		return true, nil
	}
	value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[start]), "read:"))
	return value != aiderReadPath && !aiderReadListContains(lines[start+1:end], aiderReadPath), nil
}

func aiderReadBlock(lines []string) (int, int, bool) {
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if strings.HasPrefix(trimmed, "read:") {
			start = i
			break
		}
	}
	if start < 0 {
		return 0, 0, false
	}
	end := start + 1
	scalar := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[start]), "read:")) != ""
	for end < len(lines) {
		line := lines[end]
		trimmed := strings.TrimSpace(line)
		if scalar && strings.HasPrefix(trimmed, "#") {
			break
		}
		if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(trimmed, "#") {
			break
		}
		end++
	}
	return start, end, true
}

func aiderReadListContains(lines []string, path string) bool {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-") && strings.TrimSpace(strings.TrimPrefix(trimmed, "-")) == path {
			return true
		}
	}
	return false
}

func aiderFindSequence(lines, sequence []string) int {
	for i := 0; i+len(sequence) <= len(lines); i++ {
		if strings.Join(lines[i:i+len(sequence)], "\n") == strings.Join(sequence, "\n") {
			return i
		}
	}
	return -1
}

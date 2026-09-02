package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-lineage/lineage/internal/atomicfile"
	"gopkg.in/yaml.v3"
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
	entries, sequence, err := aiderReadEntries(lines, start, end)
	if err != nil {
		return ConfigState{}, fmt.Errorf("parse %s read setting: %w", aiderConfigFile, err)
	}
	if aiderReadEntriesContain(entries, aiderReadPath) {
		return ConfigState{}, nil
	}
	var replacement []string
	if sequence && strings.HasPrefix(value, "[") {
		close := strings.LastIndex(value, "]")
		if close < 0 {
			return ConfigState{}, fmt.Errorf("parse %s read setting: flow list has no closing bracket", aiderConfigFile)
		}
		separator := ", "
		if strings.TrimSpace(value[1:close]) == "" {
			separator = ""
		}
		replacement = []string{strings.TrimSuffix(lines[start], value) + value[:close] + separator + aiderReadPath + value[close:]}
	} else if value != "" {
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
	return atomicfile.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
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
	entries, _, err := aiderReadEntries(lines, start, end)
	if err != nil {
		return false, fmt.Errorf("parse %s read setting: %w", aiderConfigFile, err)
	}
	return !aiderReadEntriesContain(entries, aiderReadPath), nil
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

func aiderReadEntries(lines []string, start, end int) ([]string, bool, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Join(lines[start:end], "\n")), &document); err != nil {
		return nil, false, err
	}
	if len(document.Content) == 0 || len(document.Content[0].Content) < 2 {
		return nil, false, fmt.Errorf("read setting has no value")
	}
	value := document.Content[0].Content[1]
	if value.Kind == yaml.SequenceNode {
		entries := make([]string, 0, len(value.Content))
		for _, entry := range value.Content {
			if entry.Kind != yaml.ScalarNode {
				return nil, true, fmt.Errorf("read list contains a non-scalar value")
			}
			entries = append(entries, entry.Value)
		}
		return entries, value.Style&yaml.FlowStyle != 0, nil
	}
	if value.Kind != yaml.ScalarNode {
		return nil, false, fmt.Errorf("read setting is not scalar or list")
	}
	return []string{value.Value}, false, nil
}

func aiderReadEntriesContain(entries []string, path string) bool {
	for _, entry := range entries {
		if entry == path {
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

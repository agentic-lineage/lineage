package provider

import (
  "fmt"
  "strings"

  "gopkg.in/yaml.v3"
)

type SkillRenderer interface {
	Render(stagedName string, source []byte) ([]byte, error)
}

type auggieSkillRenderer struct{}

func (auggieSkillRenderer) Render(
	stagedName string,
	source []byte,
) ([]byte, error) {
	body := string(source)
	metadata := map[string]any{}

	if strings.HasPrefix(body, "---\n") {
		remaining := strings.TrimPrefix(body, "---\n")
		parts := strings.SplitN(remaining, "\n---\n", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid YAML frontmatter")
		}

		if err := yaml.Unmarshal([]byte(parts[0]), &metadata); err != nil {
			return nil, fmt.Errorf("parse YAML frontmatter: %w", err)
		}

		body = strings.TrimPrefix(parts[1], "\n")
	}

	// Auggie wants the name to match the staged directory
	metadata["name"] = stagedName

	description, ok := metadata["description"].(string)
	if !ok || strings.TrimSpace(description) == "" {
		metadata["description"] = "Lineage skill " + stagedName
	}

	frontmatter, err := yaml.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("render YAML frontmatter: %w", err)
	}

	rendered := fmt.Sprintf(
		"---\n%s---\n\n%s",
		frontmatter,
		body,
	)

	return []byte(rendered), nil
}

func (p Provider) RenderSkill(
	stagedName string,
	source []byte,
) ([]byte, error) {
	if p.renderer == nil {
		return source, nil
	}

	return p.renderer.Render(stagedName, source)
}

package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/agentic-lineage/lineage/internal/inventory"
)

const (
	claudeAPIURL      = "https://api.anthropic.com/v1/messages"
	claudeModel       = "claude-opus-5"
	claudeAPIVersion  = "2023-06-01"
	claudeMaxTokens   = 8192
	claudeAPIKeyEnv   = "ANTHROPIC_API_KEY"
	claudeContentType = "application/json"
)

// ClaudeProvider calls the Anthropic Messages API directly over HTTP.
// Deliberately net/http + encoding/json rather than anthropic-sdk-go: this
// needs exactly one request/response shape — a single non-streaming call —
// which the standard library covers completely, without vendoring a large
// SDK and its transitive dependencies (this repo vendors its one existing
// dependency, gopkg.in/yaml.v3, explicitly) for one call site.
type ClaudeProvider struct {
	// APIKey overrides the ANTHROPIC_API_KEY environment variable when set.
	APIKey string
	// Client overrides http.DefaultClient when set, e.g. for a test double.
	Client *http.Client
}

// analysisSystemPrompt constrains the model to emit exactly one
// model.BehavioralModel as JSON, grounded strictly in the inventory
// evidence given in the user message. No tool use, file access, or script
// execution is granted by this call — it can only read the evidence it's
// given and respond with text — matching #104's non-goals.
const analysisSystemPrompt = `You are analyzing a Lineage source-workspace inventory (file paths, content digests, and literal citation edges between markdown files and the artifacts they mention) to produce a BehavioralModel: an ordered list of workflow Steps, each with typed Claims (inputs, outputs, skills, tools, references), setup needs, and gates.

Rules:
- Respond with exactly one JSON object matching the BehavioralModel schema (schema, name, intent, source_inventory_digest, steps, decisions) and nothing else - no prose, no markdown code fences.
- Every Claim, Step, and Decision must carry at least one evidence entry whose path and digest come from the supplied inventory, and whose note quotes the exact text supporting the claim.
- source_inventory_digest must equal the source_inventory_digest already present in the supplied inventory payload.
- If something is ambiguous and the inventory does not resolve it, emit a Decision naming exactly what's unresolved - never guess.
- Never cite evidence for a file that is not present in the supplied inventory.`

func (c ClaudeProvider) Analyze(ctx context.Context, inv inventory.Inventory) ([]byte, error) {
	apiKey := c.APIKey
	if apiKey == "" {
		apiKey = os.Getenv(claudeAPIKeyEnv)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no Claude API key: set %s or ClaudeProvider.APIKey", claudeAPIKeyEnv)
	}

	evidence, err := json.Marshal(inv)
	if err != nil {
		return nil, fmt.Errorf("marshal inventory evidence: %w", err)
	}

	reqBody, err := json.Marshal(map[string]any{
		"model":      claudeModel,
		"max_tokens": claudeMaxTokens,
		"system":     analysisSystemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": string(evidence)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeAPIURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("content-type", claudeContentType)
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", claudeAPIVersion)

	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call Claude: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Claude response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude API returned %d: %s", resp.StatusCode, body)
	}

	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse Claude response: %w", err)
	}
	for _, block := range parsed.Content {
		if block.Type == "text" {
			return []byte(stripJSONFence(block.Text)), nil
		}
	}
	return nil, fmt.Errorf("claude response had no text content")
}

// stripJSONFence removes a leading/trailing ```json or ``` fence if the
// model wrapped its response in one despite the system prompt asking for
// bare JSON - defensive, since model.ParseModel's json.Unmarshal would
// otherwise reject an otherwise-valid response over formatting alone.
func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

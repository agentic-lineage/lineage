package analysis

import (
	"context"
	"fmt"

	"github.com/agentic-lineage/lineage/internal/inventory"
)

// FixtureProvider returns a fixed, pre-recorded response regardless of inv.
// It exists so tests (and `lineage analyze --fixture`) can exercise the
// full parse/validate/fail-closed pipeline deterministically, without a
// live provider call or credentials — a hard requirement of #104's scope
// ("keep provider access optional/configurable so deterministic tests can
// use fixtures").
type FixtureProvider struct {
	// Response is the raw provider output to return, e.g. loaded from a
	// canned JSON file.
	Response []byte
	// Err, if set, is returned instead of Response, simulating a
	// provider-call failure.
	Err error
}

func (f FixtureProvider) Analyze(_ context.Context, _ inventory.Inventory) ([]byte, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if f.Response == nil {
		return nil, fmt.Errorf("fixture provider has no response configured")
	}
	return f.Response, nil
}

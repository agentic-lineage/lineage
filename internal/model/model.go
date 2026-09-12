// Package model defines a versioned, provider-neutral intermediate
// representation of a compiled workflow: ordered steps, the inputs,
// outputs, skills, tools, and references each step needs, setup
// requirements, validation gates, and unresolved/ambiguous decisions.
//
// A BehavioralModel is built from an internal/inventory.Inventory (see
// LoadInventory and NewSkeletonFromInventory) but does not itself perform
// semantic analysis: it structures evidence the inventory already found,
// it does not resolve ambiguity the inventory's literal citation matching
// could not answer. That resolution is a later, agent-assisted analysis
// stage's job — this package never guesses.
//
// Every field-level assertion in the model — a step's declared input,
// output, required skill, tool, or reference — is a Claim carrying its own
// evidence, so which evidence supports which specific assertion is never
// ambiguous. Nothing in this package executes source-workspace scripts or
// any other content it discovers; it only reads, structures, and validates.
package model

// CurrentSchema is the only BehavioralModel schema this build understands,
// following the same versioning convention as packages.Manifest.Schema and
// inventory.Inventory.Schema.
const CurrentSchema = 1

// BehavioralModel is the versioned, provider-neutral representation of one
// compiled workflow.
type BehavioralModel struct {
	Schema int    `json:"schema"`
	Name   string `json:"name"`
	Intent string `json:"intent"` // human-readable summary of workflow purpose

	// SourceInventoryDigest is a sha256 content digest over the whole
	// inventory.Inventory this model was built from (see
	// computeInventoryDigest in loader.go), computed the same deterministic way
	// packages.ComputeDigest hashes a package's content: sorted Entries by
	// Path. It lets Validate detect, in one comparison, that a model was
	// built against a different inventory snapshot than the one it is now
	// being checked against — a signal the per-Claim EvidenceRef checks
	// alone cannot give, since those only ever compare one cited file at a
	// time and cannot tell that two citations came from different runs.
	SourceInventoryDigest string `json:"source_inventory_digest"`

	Steps []Step `json:"steps"` // order is semantic — the workflow's actual sequence, not sorted

	// Decisions holds every unresolved or ambiguous piece of behavior,
	// both model-level (Refs empty) and step/field/claim-scoped (Refs
	// naming precise anchors). Sorted by ID for deterministic output.
	Decisions []Decision `json:"decisions"`
}

// Step is one ordered unit of the workflow.
type Step struct {
	ID          string `json:"id"` // stable slug, e.g. "step-2-deploy"
	Name        string `json:"name"`
	Description string `json:"description"`

	Inputs     []Claim `json:"inputs,omitempty"`
	Outputs    []Claim `json:"outputs,omitempty"`
	Skills     []Claim `json:"skills,omitempty"` // -> packages.Manifest.Requires.Skills
	Tools      []Claim `json:"tools,omitempty"`  // required scripts/executables
	References []Claim `json:"references,omitempty"`

	Setup []SetupNeed `json:"setup,omitempty"`
	Gates []Gate      `json:"gates,omitempty"` // validation checkpoints

	// Evidence supports the step's existence and description as a whole,
	// separately from any per-field Claim's own evidence. Must be
	// non-empty — Validate rejects a step asserted with no support.
	Evidence []EvidenceRef `json:"evidence"`
}

// Claim is one specific, addressable assertion inside a step — a single
// input, output, required skill, tool, or reference — carrying its own
// evidence. Without this, a step with several field values sharing one
// flat evidence list would leave a later reasoning stage guessing which
// evidence supports which specific value; Claim makes that correspondence
// explicit.
//
// Claim has no separate ID field: Value (a path or skill name) is already
// unique within one field of one step in practice — nothing asserts the
// same tool twice for the same step — so it doubles as the claim's
// identity. A synthetic ID here would just be "<StepID>:<Field>:<index>",
// pure position already implicit in where the Claim sits; storing it would
// add nothing but repeated bytes.
type Claim struct {
	Value    string        `json:"value"`
	Evidence []EvidenceRef `json:"evidence"` // must be non-empty — an asserted claim with no support is rejected
}

// SetupNeed is a local resource (file or directory) the workflow expects
// to exist, mirroring packages.Setup's shape. No separate ID for the same
// reason Claim has none: Path is already unique within a step's setup list.
type SetupNeed struct {
	Path        string        `json:"path"`
	Description string        `json:"description"`
	Kind        string        `json:"kind"` // "file" | "directory" — not derivable from Path, so kept explicit
	Evidence    []EvidenceRef `json:"evidence,omitempty"`
}

// Gate is a validation checkpoint for a step. Unlike Claim and SetupNeed,
// Gate needs a real ID: its only content field, Description, is free
// prose with no inherent uniqueness to reuse as an identity.
type Gate struct {
	ID          string        `json:"id"`
	Description string        `json:"description"`
	Evidence    []EvidenceRef `json:"evidence,omitempty"`
}

// EvidenceRef ties any claim back to a specific inventory.Entry by Path
// AND Digest — not Path alone — so a model built from one inventory
// snapshot is detectably stale if the cited file has since changed. Line
// disambiguates which citation is meant when the evidence derives from an
// inventory.Citation edge and Path cites the same target more than once.
//
// Column and Snippet are deliberately not duplicated here even though
// inventory.Citation carries them: both are fully recoverable from
// Path+Line by looking up the citing entry's Mentions in the inventory
// Validate already takes as a parameter, so copying them into every
// EvidenceRef would repeat text already stored once in the inventory,
// across every Claim and Decision that cites it — real payload cost for
// zero new information.
type EvidenceRef struct {
	Path   string `json:"path"`
	Digest string `json:"digest"` // "sha256:<hex>", copied from inventory.Entry.Digest
	Line   int    `json:"line,omitempty"`
	Note   string `json:"note,omitempty"` // the one field not derivable from the inventory — an author/agent annotation
}

// Ref addresses one of four granularities in a BehavioralModel, by which
// fields are set:
//
//   - all empty:            model-level (unscoped) — e.g. a workspace-wide
//     ambiguity not localizable to any one step.
//   - StepID only:          the whole step.
//   - StepID + Field:       that field on that step, with no claim to
//     point at yet — the field is missing or unresolved entirely.
//   - StepID + Field + Key: one specific existing claim, setup need, or
//     gate within that field.
//
// Key means whichever natural identifier that Field's items already
// carry — Claim.Value for "inputs"/"outputs"/"skills"/"tools"/"references",
// SetupNeed.Path for "setup", Gate.ID for "gates" — so Ref never needs its
// own identifier scheme duplicating one that already exists on the thing
// it points at.
type Ref struct {
	StepID string `json:"step_id,omitempty"`
	Field  string `json:"field,omitempty"` // "inputs"|"outputs"|"skills"|"tools"|"references"|"setup"|"gates"
	Key    string `json:"key,omitempty"`
}

// Decision is one unresolved or ambiguous piece of behavior. Refs is empty
// for a model-level decision, or names one or more precise anchors — a
// decision can span more than one claim.
type Decision struct {
	ID          string        `json:"id"`
	Refs        []Ref         `json:"refs,omitempty"`
	Description string        `json:"description"`
	Evidence    []EvidenceRef `json:"evidence,omitempty"`
}

// stepFields lists the valid Ref.Field values, in the order they appear on
// Step, shared by validation and the skeleton builder.
var stepFields = []string{"inputs", "outputs", "skills", "tools", "references", "setup", "gates"}

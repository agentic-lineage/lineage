package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/agentic-lineage/lineage/internal/graph"
	"gopkg.in/yaml.v3"
)

func runGraph(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || isHelpFlag(args[0]) {
		fmt.Fprintln(stdout, "usage: lineage graph list [--yaml]\n\nShow this project's local lineage graph: which packages its enabled state descends from.")
		return nil
	}
	switch args[0] {
	case "list":
		return runGraphList(args[1:], stdout, stderr)
	default:
		err := fmt.Errorf("unknown graph subcommand %q", args[0])
		fmt.Fprintln(stderr, err)
		return err
	}
}

func runGraphList(args []string, stdout, stderr io.Writer) error {
	yamlOutput := false
	for _, a := range args {
		if a == "--yaml" {
			yamlOutput = true
			continue
		}
		err := fmt.Errorf("usage: lineage graph list [--yaml]")
		fmt.Fprintln(stderr, err)
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	found, err := config.FindProjectConfig(cwd)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	records, err := graph.Load(found.Root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if yamlOutput {
		return writeGraphYAML(stdout, records)
	}

	if len(records) == 0 {
		fmt.Fprintln(stdout, "no lineage graph records")
		return nil
	}
	for _, rec := range records {
		fmt.Fprintf(stdout, "%s  %s@%s  %s  workspace=%s\n",
			rec.CreatedAt.Format("2006-01-02T15:04:05Z"),
			rec.Parent.Name, rec.Parent.Version,
			rec.Event,
			emptyValue(rec.Descendant.Workspace))
	}
	return nil
}

// GraphRecordReport is the stable --yaml representation of a graph.Record,
// following the same pattern as PackageReport (structured.go): one schema
// an integration can rely on, independent of the human-readable output's
// wording.
type GraphRecordReport struct {
	ID        string            `yaml:"id"`
	CreatedAt string            `yaml:"created_at"`
	Event     string            `yaml:"event"`
	Parent    GraphParentReport `yaml:"parent"`
	Workspace string            `yaml:"workspace,omitempty"`
	SessionID string            `yaml:"session_id,omitempty"`
	Provider  map[string]string `yaml:"provider,omitempty"`
}

type GraphParentReport struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	Digest     string `yaml:"digest,omitempty"`
	SnapshotID string `yaml:"snapshot_id,omitempty"`
}

func graphRecordReport(rec graph.Record) GraphRecordReport {
	return GraphRecordReport{
		ID:        rec.ID,
		CreatedAt: rec.CreatedAt.Format("2006-01-02T15:04:05Z"),
		Event:     rec.Event,
		Parent: GraphParentReport{
			Name:       rec.Parent.Name,
			Version:    rec.Parent.Version,
			Digest:     rec.Parent.Digest,
			SnapshotID: rec.Parent.SnapshotID,
		},
		Workspace: rec.Descendant.Workspace,
		SessionID: rec.Descendant.SessionID,
		Provider:  rec.Provider,
	}
}

func writeGraphYAML(stdout io.Writer, records []graph.Record) error {
	reports := make([]GraphRecordReport, 0, len(records))
	for _, rec := range records {
		reports = append(reports, graphRecordReport(rec))
	}
	enc := yaml.NewEncoder(stdout)
	enc.SetIndent(2)
	if err := enc.Encode(reports); err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	return enc.Close()
}

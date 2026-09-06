package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestTopLevelHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"--help"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("--help error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Package authoring:") {
		t.Fatalf("stdout = %q, want grouped command sections", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty for --help", stderr.String())
	}
	if !strings.Contains(stdout.String(), "apply packages and launch where supported") ||
		!strings.Contains(stdout.String(), "launchable providers") ||
		!strings.Contains(stdout.String(), "windsurf") {
		t.Fatalf("stdout = %q, want capability-aware provider help", stdout.String())
	}
}

func TestVersionFlag(t *testing.T) {
	for _, flag := range []string{"version", "-v", "--version"} {
		var stdout, stderr bytes.Buffer
		if err := Execute(nil, []string{flag}, nil, &stdout, &stderr); err != nil {
			t.Fatalf("%s error = %v", flag, err)
		}
		if !strings.HasPrefix(stdout.String(), "lineage ") {
			t.Fatalf("%s stdout = %q, want it to start with \"lineage \"", flag, stdout.String())
		}
	}
}

func TestPackageHelpShowsSubcommandList(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"package", "--help"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("package --help error = %v", err)
	}
	if !strings.Contains(stdout.String(), "publish <path>") {
		t.Fatalf("stdout = %q, want the subcommand list", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty for package --help", stderr.String())
	}
}

// A bare "-h" as the argument to a subcommand must show that subcommand's
// own usage instead of being treated as a path/name argument - previously
// `lineage package export -h` would try to export a package literally
// named "-h" and fail with a confusing filesystem error.
func TestPackageSubcommandHelpDoesNotTreatFlagAsArgument(t *testing.T) {
	cases := [][]string{
		{"package", "init", "-h"},
		{"package", "validate", "--help"},
		{"package", "export", "-h"},
		{"package", "import", "--help"},
		{"package", "publish", "-h"},
		{"package", "pull", "--help"},
	}
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		if err := Execute(nil, args, nil, &stdout, &stderr); err != nil {
			t.Fatalf("%v error = %v stderr=%s", args, err, stderr.String())
		}
		if !strings.HasPrefix(stdout.String(), "usage: lineage package "+args[1]) {
			t.Fatalf("%v stdout = %q, want it to start with that subcommand's usage line", args, stdout.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("%v stderr = %q, want empty", args, stderr.String())
		}
	}
}

func TestHelpFlagRecognizedAfterOtherArguments(t *testing.T) {
	cases := []struct {
		args      []string
		wantUsage string
	}{
		{[]string{"enable", "--yes", "--help"}, "usage: lineage enable"},
		{[]string{"add", "--yes", "-h"}, "usage: lineage add"},
		{[]string{"inspect", "--yaml", "--help"}, "usage: lineage inspect"},
		{
			[]string{"package", "export", ".", "-o", "out.tgz", "--help"},
			"usage: lineage package export",
		},
		{
			[]string{"package", "pull", "example", "--as", "renamed", "-h"},
			"usage: lineage package pull",
		},
	}

	for _, tc := range cases {
		var stdout, stderr bytes.Buffer

		err := Execute(nil, tc.args, nil, &stdout, &stderr)
		if err != nil {
			t.Fatalf("%v: error = %v; stderr = %q",
				tc.args, err, stderr.String())
		}

		if !strings.HasPrefix(stdout.String(), tc.wantUsage) {
			t.Fatalf("%v: stdout = %q, want prefix %q",
				tc.args, stdout.String(), tc.wantUsage)
		}

		if stderr.Len() != 0 {
			t.Fatalf("%v: stderr = %q, want empty", tc.args, stderr.String())
		}
	}
}

func TestHasHelpFlagRequiresExactToken(t *testing.T) {
	if hasHelpFlag([]string{"--helpful", "docs/--help-example"}) {
		t.Fatal("similar strings must not be interpreted as help flags")
	}
}

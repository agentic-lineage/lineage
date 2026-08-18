package runtime

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lineage-dev/lineage/internal/config"
	"github.com/lineage-dev/lineage/internal/packages"
	"github.com/lineage-dev/lineage/internal/provider"
)

type Plan struct {
	Provider     string
	ProjectRoot  string
	ConfigPath   string
	Workspace    string
	Lineage      string
	Packages     []packages.Package
	ProviderPlan provider.Plan
}

func BuildPlan(providerName, cwd, home string, args []string) (Plan, error) {
	if providerName != "claude" && providerName != "codex" {
		return Plan{}, fmt.Errorf("unknown provider %q", providerName)
	}

	found, err := config.FindProjectConfig(cwd)
	if err != nil {
		return Plan{}, err
	}

	resolved, err := packages.ResolveEnabled(home, found.Config.Workspace, found.Root, found.Config.EnabledPackages)
	if err != nil {
		return Plan{}, err
	}
	if err := packages.ValidateDependencies(resolved); err != nil {
		return Plan{}, err
	}

	providerPlan, err := provider.Resolve(providerName, home, found.Config, args)
	if err != nil {
		return Plan{}, err
	}

	return Plan{
		Provider:     providerName,
		ProjectRoot:  found.Root,
		ConfigPath:   found.Path,
		Workspace:    found.Config.Workspace,
		Lineage:      lineageString(found.Config.Workspace, resolved),
		Packages:     resolved,
		ProviderPlan: providerPlan,
	}, nil
}

func (p Plan) DryRunString() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Lineage launch plan\n")
	fmt.Fprintf(&b, "provider: %s\n", p.Provider)
	fmt.Fprintf(&b, "project_root: %s\n", p.ProjectRoot)
	fmt.Fprintf(&b, "config: %s\n", p.ConfigPath)
	fmt.Fprintf(&b, "workspace: %s\n", emptyValue(p.Workspace))
	fmt.Fprintf(&b, "lineage: %s\n", p.Lineage)
	fmt.Fprintf(&b, "real_binary: %s\n", emptyValue(p.ProviderPlan.Binary))
	fmt.Fprintf(&b, "args: %s\n", strings.Join(p.ProviderPlan.Args, " "))
	fmt.Fprintf(&b, "packages:\n")
	if len(p.Packages) == 0 {
		fmt.Fprintf(&b, "  none\n")
	}
	for _, pkg := range p.Packages {
		fmt.Fprintf(&b, "  - %s@%s\n", pkg.Manifest.Name, pkg.Manifest.Version)
		fmt.Fprintf(&b, "    path: %s\n", filepath.Clean(pkg.Path))
		fmt.Fprintf(&b, "    digest: %s\n", emptyValue(pkg.Digest))
		fmt.Fprintf(&b, "    skills: %s\n", listValue(pkg.Skills))
		fmt.Fprintf(&b, "    workflows: %s\n", listValue(pkg.Workflows))
		fmt.Fprintf(&b, "    agents: %s\n", listValue(pkg.Agents))
		fmt.Fprintf(&b, "    policies: %s\n", listValue(pkg.Policies))
		fmt.Fprintf(&b, "    capabilities:\n")
		fmt.Fprintf(&b, "      filesystem.read: %s\n", listValue(pkg.Manifest.Capabilities.Filesystem.Read))
		fmt.Fprintf(&b, "      network: %s\n", listValue(pkg.Manifest.Capabilities.Network))
	}
	return b.String()
}

func lineageString(workspace string, pkgs []packages.Package) string {
	parts := []string{"user"}
	if workspace != "" {
		parts = append(parts, "workspace:"+workspace)
	}
	parts = append(parts, "project")
	for _, pkg := range pkgs {
		parts = append(parts, pkg.Manifest.Name+"@"+pkg.Manifest.Version)
	}
	return strings.Join(parts, " -> ")
}

func listValue(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func emptyValue(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

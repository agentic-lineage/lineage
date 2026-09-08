package packages

import (
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/yaml.v3"
)

var allowedMCPFields = map[string]bool{
	"name": true, "transport": true, "command": true, "args": true, "url": true, "auth": true,
}

// validateMCPManifestFields rejects unmodelled MCP fields before the typed
// manifest decoder can discard them. In particular, this prevents a package
// from smuggling credential-bearing headers, tokens, or environment values
// into a surface that promises not to distribute them.
func validateMCPManifestFields(data []byte) error {
	var raw struct {
		Dependencies struct {
			MCP []map[string]interface{} `yaml:"mcp"`
		} `yaml:"dependencies"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	for i, server := range raw.Dependencies.MCP {
		for field := range server {
			if !allowedMCPFields[field] {
				return fmt.Errorf("dependencies.mcp[%d].%s is not supported; credentials and provider-specific configuration must stay receiver-local", i, field)
			}
		}
	}
	return nil
}

// ValidateMCPDependencies validates package-level MCP declarations without
// contacting a server or executing a command. Credentials are intentionally
// not representable in MCPDependency; auth can only say that the receiver must
// supply it.
func ValidateMCPDependencies(deps []MCPDependency, networkCapabilities []string) []string {
	var errors []string
	seen := map[string]bool{}
	for i, dep := range deps {
		prefix := fmt.Sprintf("dependencies.mcp[%d]", i)
		if err := validateIdentifier("name", "lineage.yaml", dep.Name); err != nil {
			errors = append(errors, prefix+": "+err.Error())
			continue
		}
		if seen[dep.Name] {
			errors = append(errors, fmt.Sprintf("%s: duplicate MCP server name %q", prefix, dep.Name))
		}
		seen[dep.Name] = true

		switch dep.Transport {
		case "stdio":
			if strings.TrimSpace(dep.Command) == "" {
				errors = append(errors, prefix+": stdio transport requires command")
			}
			if dep.URL != "" {
				errors = append(errors, prefix+": stdio transport must not declare url")
			}
		case "streamable-http", "sse":
			if strings.TrimSpace(dep.Command) != "" || len(dep.Args) > 0 {
				errors = append(errors, prefix+": remote transport must not declare command or args")
			}
			u, err := url.Parse(dep.URL)
			if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
				errors = append(errors, prefix+": remote transport requires an absolute https url without embedded credentials")
			} else if !networkCapabilityAllows(networkCapabilities, u.Hostname()) {
				errors = append(errors, fmt.Sprintf("%s: remote host %q must be declared in capabilities.network", prefix, u.Hostname()))
			}
		default:
			errors = append(errors, fmt.Sprintf("%s: transport %q is unsupported (use stdio, streamable-http, or sse)", prefix, dep.Transport))
		}
		if dep.Auth != "" && dep.Auth != "none" && dep.Auth != "receiver" {
			errors = append(errors, fmt.Sprintf("%s: auth %q is unsupported (use none or receiver)", prefix, dep.Auth))
		}
	}
	return errors
}

func networkCapabilityAllows(capabilities []string, host string) bool {
	for _, capability := range capabilities {
		if capability == "*" || strings.EqualFold(capability, host) {
			return true
		}
	}
	return false
}

// ValidateMCPProviderSupport prevents a declared server from being silently
// dropped. The current Claude and Codex adapters only materialize package
// assets; neither owns a safe merge boundary for native MCP configuration yet.
// Until that adapter work exists, a package remains inspectable but cannot run
// through Lineage with MCP dependencies enabled.
func ValidateMCPProviderSupport(providerName string, pkgs []Package) error {
	for _, pkg := range pkgs {
		if len(pkg.Manifest.Dependencies.MCP) > 0 {
			return fmt.Errorf("provider %q does not yet support MCP materialization required by package %q; refusing to silently drop %d declared MCP server(s)", providerName, pkg.Manifest.Name, len(pkg.Manifest.Dependencies.MCP))
		}
	}
	return nil
}

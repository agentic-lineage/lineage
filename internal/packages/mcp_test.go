package packages

import "testing"

func TestValidateMCPDependencies(t *testing.T) {
	tests := []struct {
		name    string
		dep     MCPDependency
		network []string
		wantErr bool
	}{
		{"stdio", MCPDependency{Name: "github", Transport: "stdio", Command: "npx", Args: []string{"-y", "server"}, Auth: "receiver"}, nil, false},
		{"remote", MCPDependency{Name: "docs", Transport: "streamable-http", URL: "https://mcp.example.com/mcp", Auth: "receiver"}, []string{"mcp.example.com"}, false},
		{"remote requires capability", MCPDependency{Name: "docs", Transport: "sse", URL: "https://mcp.example.com/sse"}, nil, true},
		{"embedded credential", MCPDependency{Name: "bad", Transport: "sse", URL: "https://token@example.com/sse"}, []string{"example.com"}, true},
		{"stdio needs command", MCPDependency{Name: "bad", Transport: "stdio"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateMCPDependencies([]MCPDependency{tt.dep}, tt.network)
			if (len(got) > 0) != tt.wantErr {
				t.Fatalf("errors = %v, wantErr %v", got, tt.wantErr)
			}
		})
	}
}

func TestValidateMCPProviderSupportRefusesSilentDrop(t *testing.T) {
	pkg := Package{Manifest: Manifest{
		Name: "mcp-pack",
		Dependencies: Dependencies{MCP: []MCPDependency{{
			Name: "docs", Transport: "stdio", Command: "npx",
		}}},
	}}
	if err := ValidateMCPProviderSupport("claude", []Package{pkg}); err == nil {
		t.Fatal("ValidateMCPProviderSupport() error = nil, want unsupported adapter error")
	}
}

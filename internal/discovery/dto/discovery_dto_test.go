package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"vpsmonitoring-backend/internal/discovery/dto"
)

// Agent representation mirroring the actual vpsmonitoring-agent struct
// to prove contract compatibility without cyclic module imports.
type AgentWebsiteCandidate struct {
	ID               string                    `json:"id,omitempty"`
	Domains          []AgentWebDomainCandidate `json:"domains"`
	DocumentRoot     string                    `json:"document_root,omitempty"`
	WebServerKind    string                    `json:"web_server_kind"`
	Source           string                    `json:"source"`
	ListenPorts      []int                     `json:"listen_ports,omitempty"`
	ProxyPassTargets []string                  `json:"proxy_pass_targets,omitempty"`
	FpmSocketPaths   []string                  `json:"fpm_socket_paths,omitempty"`
	Confidence       string                    `json:"confidence"`
	Evidence         []AgentEvidence           `json:"evidence"`
}

type AgentWebDomainCandidate struct {
	Name      string `json:"name"`
	IsPrimary bool   `json:"is_primary"`
}

type AgentEvidence struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type AgentDiscoveryPayload struct {
	Websites []AgentWebsiteCandidate `json:"websites,omitempty"`
}

func TestAgentToBackendContract(t *testing.T) {
	agentPayload := AgentDiscoveryPayload{
		Websites: []AgentWebsiteCandidate{
			{
				Domains: []AgentWebDomainCandidate{
					{Name: "example.com", IsPrimary: true},
					{Name: "www.example.com", IsPrimary: false},
				},
				DocumentRoot:  "/var/www/example",
				WebServerKind: "apache",
				Confidence:    "HIGH",
				Source:        "apache_config",
				Evidence: []AgentEvidence{
					{Kind: "config_file", Value: "/etc/apache2/sites-enabled/000-default.conf"},
				},
			},
			{
				// Missing primary flag should fallback to Domains[0] via service logic (verified later in service tests),
				// but here we just verify unmarshaling works.
				Domains: []AgentWebDomainCandidate{
					{Name: "fallback.com", IsPrimary: false},
				},
				DocumentRoot:  "/var/www/fallback",
				WebServerKind: "nginx",
				Confidence:    "LOW",
				Source:        "nginx_config",
			},
			{
				// Empty domains array
				Domains:       []AgentWebDomainCandidate{},
				DocumentRoot:  "/var/www/empty",
				WebServerKind: "caddy",
				Confidence:    "LOW",
				Source:        "caddy_config",
			},
		},
	}

	// 1. Serialize Agent Payload to JSON
	agentJSON, err := json.Marshal(agentPayload)
	require.NoError(t, err)

	// 2. Unmarshal into Backend DTO
	var backendPayload dto.DiscoveryPayloadDTO
	err = json.Unmarshal(agentJSON, &backendPayload)
	require.NoError(t, err)

	// 3. Verify mappings
	require.Len(t, backendPayload.Websites, 3)

	// First website
	wc1 := backendPayload.Websites[0]
	assert.Len(t, wc1.Domains, 2)
	assert.Equal(t, "example.com", wc1.Domains[0].DomainName)
	assert.True(t, wc1.Domains[0].IsPrimary)
	assert.Equal(t, "www.example.com", wc1.Domains[1].DomainName)
	assert.False(t, wc1.Domains[1].IsPrimary)
	assert.Equal(t, "/var/www/example", wc1.DocumentRoot)
	assert.Equal(t, "apache", wc1.WebServerKind)
	assert.Equal(t, string(dto.ConfidenceHigh), string(wc1.Confidence))
	assert.Equal(t, "apache_config", wc1.Source)

	// Second website
	wc2 := backendPayload.Websites[1]
	assert.Len(t, wc2.Domains, 1)
	assert.Equal(t, "fallback.com", wc2.Domains[0].DomainName)
	assert.False(t, wc2.Domains[0].IsPrimary)

	// Third website
	wc3 := backendPayload.Websites[2]
	assert.Len(t, wc3.Domains, 0)
}

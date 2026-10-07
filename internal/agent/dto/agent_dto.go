package dto

// RegisterAgentRequest defines the payload sent by the agent script during onboarding
type RegisterAgentRequest struct {
	InstallationToken string `json:"installation_token"`
	Hostname          string `json:"hostname"`
	IPAddress         string `json:"ip_address"`
	OS                string `json:"os"`
	Architecture      string `json:"architecture"`
	AgentVersion      string `json:"agent_version"`
}

// RegisterAgentResponse defines the one-time credential delivery payload
type RegisterAgentResponse struct {
	AgentID    string `json:"agent_id"`
	ServerID   int64  `json:"server_id"`
	Credential string `json:"credential"`
	Status     string `json:"status"`
}

// HeartbeatRequest defines the payload periodically reported by the Go Agent
type HeartbeatRequest struct {
	Timestamp    string `json:"timestamp"`
	AgentVersion string `json:"agent_version"`
}

// HeartbeatResponse defines the response returned after a successful heartbeat update
type HeartbeatResponse struct {
	AgentID  string `json:"agent_id"`
	ServerID int64  `json:"server_id"`
	Status   string `json:"status"`
	LastSeen string `json:"last_seen"`
}

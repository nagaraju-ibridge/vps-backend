package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"gofr.dev/pkg/gofr"

	"vpsmonitoring-backend/internal/agent/service"
)

type agentContextKey string

const (
	AgentIdentityContextKey agentContextKey = "vpspulse_agent_identity"
)

// AgentIdentity contains authenticated agent and associated server metadata
type AgentIdentity struct {
	AgentID  string `json:"agent_id"`
	ServerID int64  `json:"server_id"`
	Status   string `json:"status"`
}

// AgentAuthMiddleware validates raw agent credentials via Authorization: Bearer <AGENT_CREDENTIAL>
// and stores the authenticated AgentIdentity in request context.
func AgentAuthMiddleware(agentService service.AgentService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// 1. Only enforce this middleware on agent-only endpoints
			// /api/v1/agent/register is public (uses installation token in JSON body)
			if !(strings.HasPrefix(path, "/api/v1/agent/") || strings.HasPrefix(path, "/api/v1/agents/")) || path == "/api/v1/agent/register" {
				next.ServeHTTP(w, r)
				return
			}

			// 2. Read Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				respondAgentUnauthorized(w, "Authorization header missing")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
				respondAgentUnauthorized(w, "Invalid authorization header format, expected 'Bearer <credential>'")
				return
			}

			rawCredential := strings.TrimSpace(parts[1])

			// 3. Authenticate agent using service layer
			agent, err := agentService.AuthenticateAgent(r.Context(), rawCredential)
			if err != nil {
				// Avoid revealing detailed authentication failure messages that could help credential enumeration
				respondAgentUnauthorized(w, "Unauthorized: invalid agent credential")
				return
			}

			// 4. Inject typed AgentIdentity into request context
			identity := &AgentIdentity{
				AgentID:  agent.AgentID,
				ServerID: agent.ServerID,
				Status:   agent.Status,
			}
			ctx := context.WithValue(r.Context(), AgentIdentityContextKey, identity)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetAgentIdentity retrieves AgentIdentity from the GoFr context for downstream handlers
func GetAgentIdentity(ctx *gofr.Context) (*AgentIdentity, error) {
	if ctx == nil {
		return nil, errors.New("context is nil")
	}

	val := ctx.Context.Value(AgentIdentityContextKey)
	if val == nil {
		return nil, errors.New("unauthenticated request: agent identity not found in context")
	}

	identity, ok := val.(*AgentIdentity)
	if !ok || identity == nil {
		return nil, errors.New("invalid agent identity format in context")
	}

	return identity, nil
}

func respondAgentUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"error":{"message":"%s"}}`, message)))
}

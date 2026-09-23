package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/orchestration/models"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// orchestrationAPIPrefix is the backend API every broker tool calls. Each
// call carries the run's coordinator JWT and is authorized server-side.
const orchestrationAPIPrefix = "/api/v1/orchestration"

// runOrchestratorMCP serves the workspace coordinator broker over stdio. It is
// the only MCP server a broker-restricted coordinator receives.
func runOrchestratorMCP() int {
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	if err := server.ServeStdio(newOrchestratorMCP(client)); err != nil {
		cliError("orchestrator broker stopped: %v", err)
		return 1
	}
	return 0
}

func newOrchestratorMCP(client *kandevClient) *server.MCPServer {
	s := server.NewMCPServer(config.BrokerMCPServerName, "1.0.0", server.WithToolCapabilities(false))
	for _, definition := range models.WorkspaceBrokerTools() {
		options := []mcp.ToolOption{mcp.WithDescription(definition.Description), mcp.WithObject("query", mcp.Description("Optional query parameters."))}
		if strings.Contains(definition.Path, ":id") {
			options = append(options, mcp.WithString("id", mcp.Required()))
		}
		if definition.Method == http.MethodGet {
			options = append(options, mcp.WithReadOnlyHintAnnotation(true))
		} else {
			options = append(options, mcp.WithObject("request", mcp.Required(), mcp.Description("Native request body. Runtime authorization is checked by Kandev.")))
		}
		s.AddTool(mcp.NewTool(definition.Name, options...), func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return callOrchestratorBroker(client, definition, request.GetArguments())
		})
	}
	return s
}

func callOrchestratorBroker(client *kandevClient, definition models.WorkspaceBrokerTool, args map[string]any) (*mcp.CallToolResult, error) {
	path := definition.Path
	if strings.Contains(path, ":id") {
		id, _ := args["id"].(string)
		if id == "" || len(id) > 200 || strings.ContainsAny(id, "/\\?#%") || id == "." || id == ".." {
			return mcp.NewToolResultError("invalid resource id"), nil
		}
		path = strings.Replace(path, ":id", url.PathEscape(id), 1)
	}
	if query, ok := args["query"].(map[string]any); ok {
		values := url.Values{}
		for key, value := range query {
			values.Set(key, fmt.Sprint(value))
		}
		path += "?" + values.Encode()
	}
	data, status, err := brokerRequestClient(client, definition, args).do(definition.Method, orchestrationAPIPrefix+path, args["request"])
	if err != nil {
		return mcp.NewToolResultError("broker transport unavailable; a write may have an unknown outcome"), nil
	}
	if status < 200 || status >= 300 {
		if strings.TrimSpace(string(data)) == "" {
			return mcp.NewToolResultError(fmt.Sprintf("Kandev returned HTTP %d (%s)", status, http.StatusText(status))), nil
		}
		return mcp.NewToolResultError(string(data)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// Session startup can outlast the normal read deadline. Preserve the shared
// client and never retry a mutation whose response was lost.
func brokerRequestClient(client *kandevClient, definition models.WorkspaceBrokerTool, args map[string]any) *kandevClient {
	request, _ := args["request"].(map[string]any)
	action, _ := request["action"].(string)
	if definition.Name != "manage_task" || (action != "start" && action != "message") {
		return client
	}
	scoped := *client
	transport := *client.http
	transport.Timeout = 2 * time.Minute
	scoped.http = &transport
	return &scoped
}

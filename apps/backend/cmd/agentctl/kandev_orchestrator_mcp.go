package main

import (
	"context"
	"encoding/json"
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
		s.AddTool(mcp.NewTool(definition.Name, brokerToolOptions(definition)...), func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if definition.Batch {
				if ids, ok := request.GetArguments()["ids"]; ok {
					return callOrchestratorBatch(client, definition, ids, request.GetArguments())
				}
			}
			return callOrchestratorBroker(client, definition, request.GetArguments())
		})
	}
	return s
}

// brokerToolOptions publishes each tool's typed query and request schema.
func brokerToolOptions(definition models.WorkspaceBrokerTool) []mcp.ToolOption {
	options := []mcp.ToolOption{mcp.WithDescription(definition.Description)}
	if definition.Query != nil {
		options = append(options, mcp.WithObject("query", mcp.Description("Optional query parameters."), mcp.Properties(definition.Query)))
	}
	if strings.Contains(definition.Path, ":id") {
		if definition.Batch {
			options = append(options,
				mcp.WithString("id", mcp.Description("Task id. Omit when ids is given.")),
				mcp.WithArray("ids", mcp.Description("Task ids to apply the same request to."), mcp.WithStringItems(), mcp.MaxItems(models.BrokerBatchLimit)))
		} else {
			options = append(options, mcp.WithString("id", mcp.Required()))
		}
	}
	switch definition.Method {
	case http.MethodGet:
		options = append(options, mcp.WithReadOnlyHintAnnotation(true))
	case http.MethodDelete:
		options = append(options, mcp.WithDestructiveHintAnnotation(true))
	default:
		request := []mcp.PropertyOption{mcp.Required(), mcp.Description("Request body.")}
		if definition.Request != nil {
			request = append(request, mcp.Properties(definition.Request))
		}
		if len(definition.RequestRequired) > 0 {
			request = append(request, requiredProperties(definition.RequestRequired))
		}
		options = append(options, mcp.WithObject("request", request...))
	}
	return options
}

func requiredProperties(names []string) mcp.PropertyOption {
	return func(schema map[string]any) {
		schema["required"] = names
	}
}

// callOrchestratorBatch applies one request to each task in ids through the
// single-task endpoint, so every change is authorized on its own, and reports
// a result per task.
func callOrchestratorBatch(client *kandevClient, definition models.WorkspaceBrokerTool, raw any, args map[string]any) (*mcp.CallToolResult, error) {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 || len(values) > models.BrokerBatchLimit {
		return mcp.NewToolResultError(fmt.Sprintf("ids must list 1 to %d task ids", models.BrokerBatchLimit)), nil
	}
	if definition.Name == "manage_task" {
		request, _ := args["request"].(map[string]any)
		if action, _ := request["action"].(string); !models.BatchActions[action] {
			return mcp.NewToolResultError("ids supports move, archive, adopt, assign, start and stop; call other actions per task"), nil
		}
	}
	results := make([]map[string]any, 0, len(values))
	failed := 0
	for _, value := range values {
		id, _ := value.(string)
		single := map[string]any{"id": id, "request": args["request"]}
		result, _ := callOrchestratorBroker(client, definition, single)
		row := map[string]any{"id": id, "ok": !result.IsError}
		if text := brokerResultText(result); result.IsError {
			failed++
			row["error"] = text
		}
		results = append(results, row)
	}
	data, err := json.Marshal(map[string]any{"results": results, "failed": failed})
	if err != nil {
		return nil, err
	}
	if failed == len(values) {
		return mcp.NewToolResultError(string(data)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func brokerResultText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if text, ok := content.(mcp.TextContent); ok {
			return text.Text
		}
	}
	return ""
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

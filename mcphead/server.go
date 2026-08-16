// Package mcphead is the growth-ecosystem head: it exposes device capabilities
// as MCP (Model Context Protocol) tools, so LLM agents (Claude/Cursor/...)
// can discover and invoke them through the SAME sdk.ConnectivityAPI.
//
// Each capability becomes one tool: {name, description, input_schema} is the
// capability's tool definition plus a required `device` parameter. Tool calls
// go through the full Core governance (policy → approval → lock → execute →
// verify), so the agent inherits deny-by-default, risk approval and audit.
package mcphead

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/sdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Options configures the MCP head.
type Options struct {
	Principal  string
	SessionTTL time.Duration
}

// Build creates an MCP server whose tools are the union of capabilities across
// all discovered devices.
func Build(api sdk.ConnectivityAPI, opts Options) (*server.MCPServer, error) {
	if opts.Principal == "" {
		opts.Principal = "mcp"
	}
	if opts.SessionTTL <= 0 {
		opts.SessionTTL = time.Minute
	}

	s := server.NewMCPServer("hardware-channel", "1.0.0")

	devices, err := api.Discover(context.Background())
	if err != nil {
		return nil, err
	}

	seen := map[domain.CapabilityName]domain.Capability{}
	for _, d := range devices {
		caps, err := api.DescribeCapabilities(d.ID)
		if err != nil {
			continue
		}
		for _, c := range caps {
			if _, ok := seen[c.Name]; !ok {
				seen[c.Name] = c
			}
		}
	}

	for _, cap := range seen {
		cap := cap
		s.AddTool(newTool(cap), toolHandler(api, cap, opts))
	}
	return s, nil
}

// Serve runs the MCP server over stdio (the standard MCP transport).
func Serve(s *server.MCPServer) error { return server.ServeStdio(s) }

func newTool(cap domain.Capability) mcp.Tool {
	schema, _ := json.Marshal(toolSchema(cap))
	tool := mcp.NewToolWithRawSchema(string(cap.Name), cap.Description, schema)
	// Risk annotation: HIGH/CRITICAL capabilities are marked destructive so the
	// LLM treats them with care (our governance is still the real gate).
	dest := cap.RiskLevel == domain.RiskHigh || cap.RiskLevel == domain.RiskCritical
	tool.Annotations.DestructiveHint = &dest
	tool.Annotations.IdempotentHint = &cap.Idempotent
	return tool
}

// toolSchema augments the capability's input schema with a required `device`
// parameter identifying the target device.
func toolSchema(cap domain.Capability) map[string]any {
	base := map[string]any{"type": "object", "properties": map[string]any{}}
	if cap.InputSchema != nil {
		if b, err := json.Marshal(cap.InputSchema); err == nil {
			var m map[string]any
			if json.Unmarshal(b, &m) == nil {
				base = m
			}
		}
	}
	props, _ := base["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
	}
	props["device"] = map[string]any{"type": "string", "description": "目标设备 serial 或 ID"}
	base["properties"] = props
	req, _ := base["required"].([]any)
	base["required"] = append(req, "device")
	return base
}

func toolHandler(api sdk.ConnectivityAPI, cap domain.Capability, opts Options) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleCall(ctx, api, cap, opts, req.GetArguments())
	}
}

// handleCall executes a capability on behalf of an MCP tool call.
func handleCall(ctx context.Context, api sdk.ConnectivityAPI, cap domain.Capability, opts Options, args map[string]any) (*mcp.CallToolResult, error) {
	devRef, _ := args["device"].(string)
	if devRef == "" {
		return mcp.NewToolResultError("缺少 device 参数（目标设备 serial/ID）"), nil
	}
	d, err := findDevice(api, devRef)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	params := map[string]string{}
	for k, v := range args {
		if k == "device" {
			continue
		}
		switch t := v.(type) {
		case string:
			params[k] = t
		default:
			params[k] = fmt.Sprintf("%v", t)
		}
	}

	sess, err := api.CreateSession(opts.Principal, d.ID, opts.SessionTTL)
	if err != nil {
		return mcp.NewToolResultError("会话失败: " + err.Error()), nil
	}
	res, err := api.Execute(ctx, domain.OperationRequest{
		Capability: cap.Name, Target: d.ID, SessionID: sess.ID, Parameters: params,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("%s: %v", res.State, err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("状态: %s\n%s", res.State, res.Output)), nil
}

func findDevice(api sdk.ConnectivityAPI, ref string) (*domain.Device, error) {
	devices := api.ListDevices()
	if len(devices) == 0 {
		devices, _ = api.Discover(context.Background())
	}
	for _, d := range devices {
		if string(d.ID) == ref || d.Serial == ref ||
			strings.HasPrefix(string(d.ID), ref) || strings.HasPrefix(d.Serial, ref) {
			return d, nil
		}
	}
	return nil, fmt.Errorf("未找到设备 %q", ref)
}

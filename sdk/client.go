// Package sdk is the high-level Unified API client used by Embedded Loop,
// Agents, CLIs and tests (Design docs 03, 15). It exposes Devices,
// Capabilities, Sessions, Operations and Events as stable handles — never the
// Plugin SPI or protocol internals.
package sdk

import (
	"context"
	"time"

	"example.com/embedded-loop-channel/core/artifact"
	"example.com/embedded-loop-channel/core/discovery"
	"example.com/embedded-loop-channel/core/event"
	"example.com/embedded-loop-channel/core/operation"
	"example.com/embedded-loop-channel/core/registry"
	"example.com/embedded-loop-channel/core/resolver"
	"example.com/embedded-loop-channel/core/security"
	"example.com/embedded-loop-channel/core/session"
	"example.com/embedded-loop-channel/domain"
	pluginregistry "example.com/embedded-loop-channel/plugin/registry"
	pluginsdk "example.com/embedded-loop-channel/plugin/sdk"
)

// Client is the Unified API entry point.
type Client struct {
	discovery     *discovery.Service
	engine        *operation.Engine
	reg           *registry.Registry
	resolver      *resolver.Resolver
	plugins       *pluginregistry.Registry
	sessions      *session.Manager
	bus           *event.Bus
	policy        *security.Policy
	approval      *security.ApprovalService
	authenticator *security.Authenticator
	secrets       *security.SecretStore
	artifacts     *artifact.Service
}

// Deps wires the Client's collaborators.
type Deps struct {
	Discovery     *discovery.Service
	Engine        *operation.Engine
	Registry      *registry.Registry
	Resolver      *resolver.Resolver
	Plugins       *pluginregistry.Registry
	Sessions      *session.Manager
	Bus           *event.Bus
	Policy        *security.Policy
	Approval      *security.ApprovalService
	Authenticator *security.Authenticator
	Secrets       *security.SecretStore
	Artifacts     *artifact.Service
}

// New builds a Client.
func New(d Deps) *Client {
	return &Client{
		discovery:     d.Discovery,
		engine:        d.Engine,
		reg:           d.Registry,
		resolver:      d.Resolver,
		plugins:       d.Plugins,
		sessions:      d.Sessions,
		bus:           d.Bus,
		policy:        d.Policy,
		approval:      d.Approval,
		authenticator: d.Authenticator,
		secrets:       d.Secrets,
		artifacts:     d.Artifacts,
	}
}

// Discover scans sources and registers devices/channels.
func (c *Client) Discover(ctx context.Context) ([]*domain.Device, error) {
	return c.discovery.Discover(ctx)
}

// Refresh detects hotplug changes and returns added/removed devices.
func (c *Client) Refresh(ctx context.Context) (added, removed []*domain.Device, err error) {
	return c.discovery.Refresh(ctx)
}

// ListDevices returns all known devices.
func (c *Client) ListDevices() []*domain.Device { return c.reg.ListDevices() }

// GetDevice returns a device by ID.
func (c *Client) GetDevice(id domain.DeviceID) (*domain.Device, error) {
	if d, ok := c.reg.GetDevice(id); ok {
		return d, nil
	}
	return nil, domain.NewError(domain.CodeUnknown, domain.CategoryDiscovery, "device not found: "+string(id))
}

// ListCapabilities returns the union of capabilities across a device's
// channels, in stable order.
func (c *Client) ListCapabilities(deviceID domain.DeviceID) ([]domain.CapabilityName, error) {
	channels := c.reg.ChannelsByDevice(deviceID)
	if len(channels) == 0 {
		return nil, domain.NewError(domain.CodeUnknown, domain.CategoryDiscovery, "device has no channels")
	}
	seen := map[domain.CapabilityName]bool{}
	var out []domain.CapabilityName
	for _, ch := range channels {
		for _, cap := range ch.Capabilities {
			if !seen[cap] {
				seen[cap] = true
				out = append(out, cap)
			}
		}
	}
	return out, nil
}

// DescribeCapabilities returns the full capability descriptors (name +
// description + input/output schema) of a device, not just names. This is the
// LLM/CLI-facing introspection: each entry is exactly a tool definition.
func (c *Client) DescribeCapabilities(deviceID domain.DeviceID) ([]domain.Capability, error) {
	channels := c.reg.ChannelsByDevice(deviceID)
	if len(channels) == 0 {
		return nil, domain.NewError(domain.CodeUnknown, domain.CategoryDiscovery, "device has no channels")
	}
	seen := map[domain.CapabilityName]bool{}
	var out []domain.Capability
	for _, ch := range channels {
		plugin, err := c.plugins.Get(ch.PluginID)
		if err != nil {
			continue
		}
		for _, cap := range plugin.Capabilities(ch) {
			if !seen[cap.Name] {
				seen[cap.Name] = true
				out = append(out, cap)
			}
		}
	}
	return out, nil
}

// CreateSession opens a Session for principal bound to device.
func (c *Client) CreateSession(principal string, deviceID domain.DeviceID, ttl time.Duration) (*domain.Session, error) {
	return c.sessions.Create(principal, deviceID, ttl)
}

// Execute creates and runs an Operation, returning the final result. The
// operation ID is available on the result.
func (c *Client) Execute(ctx context.Context, req domain.OperationRequest) (*domain.OperationResult, error) {
	op, err := c.engine.Create(req)
	if err != nil {
		return nil, err
	}
	return c.engine.Execute(ctx, op.ID)
}

// Watch returns a subscription for events matching filter.
func (c *Client) Watch(filter event.Filter) *event.Subscription {
	return c.bus.Subscribe(filter)
}

// Events returns stored events matching filter.
func (c *Client) Events(filter event.Filter) []domain.Event { return c.bus.Events(filter) }

// OpenStream resolves a channel for the capability and opens a live device
// stream (console/log) over a streaming-capable plugin. Streaming is a
// continuous read, not an Operation (no lock/policy/state machine).
func (c *Client) OpenStream(ctx context.Context, deviceID domain.DeviceID, capability domain.CapabilityName) (pluginsdk.Stream, error) {
	res, err := c.resolver.ResolveChannel(deviceID, capability, "")
	if err != nil {
		return nil, err
	}
	plugin, err := c.plugins.Get(res.Channel.PluginID)
	if err != nil {
		return nil, domain.NewError(domain.CodeInternal, domain.CategoryInternal, err.Error())
	}
	streamer, ok := plugin.(pluginsdk.Streamer)
	if !ok {
		return nil, domain.NewError(domain.CodeUnsupportedCap, domain.CategoryValidation,
			"plugin "+res.Channel.PluginID+" does not support streaming")
	}
	return streamer.Stream(ctx, res.Channel, pluginsdk.StreamRequest{Capability: capability})
}

// CreateOperation registers an operation without executing it (used by
// approval and advanced flows).
func (c *Client) CreateOperation(req domain.OperationRequest) (*domain.Operation, error) {
	return c.engine.Create(req)
}

// ExecuteOperation runs a previously-created operation.
func (c *Client) ExecuteOperation(ctx context.Context, opID domain.OperationID) (*domain.OperationResult, error) {
	return c.engine.Execute(ctx, opID)
}

// Start creates an operation and starts it asynchronously, returning the
// operation handle immediately (long-operation path: poll with Wait).
func (c *Client) Start(ctx context.Context, req domain.OperationRequest) (*domain.Operation, error) {
	return c.engine.Start(ctx, req)
}

// Wait polls an operation to a terminal state and returns its result.
func (c *Client) Wait(ctx context.Context, opID domain.OperationID, interval time.Duration) (*domain.OperationResult, error) {
	return c.engine.Wait(ctx, opID, interval)
}

// Cancel requests cooperative cancellation of a running operation.
func (c *Client) Cancel(ctx context.Context, opID domain.OperationID) error {
	return c.engine.Cancel(ctx, opID)
}

// GetEvidence returns recorded evidence for a result.
func (c *Client) GetEvidence(id domain.EvidenceID) (*domain.Evidence, bool) {
	return c.engine.GetEvidence(id)
}

// GetArtifact returns a recorded artifact.
func (c *Client) GetArtifact(id domain.ArtifactID) (*domain.Artifact, bool) {
	return c.engine.GetArtifact(id)
}

// ArtifactContent returns an artifact's object bytes.
func (c *Client) ArtifactContent(id domain.ArtifactID) ([]byte, bool) {
	if c.artifacts == nil {
		return nil, false
	}
	return c.artifacts.Content(id)
}

// VerifyArtifact recomputes an artifact's checksum.
func (c *Client) VerifyArtifact(id domain.ArtifactID) (bool, error) {
	if c.artifacts == nil {
		return false, nil
	}
	return c.artifacts.Verify(id)
}

// ListArtifacts returns all stored artifact metadata.
func (c *Client) ListArtifacts() []*domain.Artifact {
	if c.artifacts == nil {
		return nil
	}
	return c.artifacts.List()
}

// Approve records an approval for a high-risk operation.
func (c *Client) Approve(opID domain.OperationID) { c.approval.Approve(opID) }

// Grant authorizes principal to use capability across all devices.
func (c *Client) Grant(principal string, cap domain.CapabilityName) {
	c.policy.Grant(principal, cap)
}

// GrantScope authorizes principal to use capability on a SPECIFIC device
// (resource scope).
func (c *Client) GrantScope(principal string, cap domain.CapabilityName, device domain.DeviceID) {
	c.policy.GrantScope(principal, cap, device)
}

// PreApprove authorizes principal to bypass the approval gate for a
// HIGH/CRITICAL capability.
func (c *Client) PreApprove(principal string, cap domain.CapabilityName) {
	c.policy.PreApprove(principal, cap)
}

// Authenticate resolves a bearer token to an Identity (authn).
func (c *Client) Authenticate(ctx context.Context, token string) (security.Identity, error) {
	if c.authenticator == nil {
		return security.Identity{}, domain.NewError(domain.CodeInternal, domain.CategoryInternal, "no authenticator configured")
	}
	return c.authenticator.Authenticate(ctx, token)
}

// ResolveSecret resolves a SecretRef to its value (use at point-of-use only).
func (c *Client) ResolveSecret(ref security.SecretRef) (string, error) {
	if c.secrets == nil {
		return "", domain.NewError(domain.CodeInternal, domain.CategoryInternal, "no secret store configured")
	}
	return c.secrets.Resolve(context.Background(), ref)
}

// Redact replaces known secret values in s with "***" (before logging).
func (c *Client) Redact(s string) string {
	if c.secrets == nil {
		return s
	}
	return c.secrets.Redact(s)
}

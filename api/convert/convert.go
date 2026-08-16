// Package convert maps between the in-process domain model and the canonical
// protobuf wire contract generated from api/proto. The domain model remains
// the source of truth in-process; the proto messages are the stable boundary
// contract (Design docs 03, 14).
package convert

import (
	"encoding/json"
	"time"

	channelv1 "example.com/embedded-loop-channel/api/gen/channelv1"
	"example.com/embedded-loop-channel/domain"
)

// ---------------------------------------------------------------------------
// Enums: domain -> proto
// ---------------------------------------------------------------------------

var deviceStateToProto = map[domain.DeviceState]channelv1.DeviceState{
	domain.DeviceStateOnline:      channelv1.DeviceState_DEVICE_STATE_ONLINE,
	domain.DeviceStateOffline:     channelv1.DeviceState_DEVICE_STATE_OFFLINE,
	domain.DeviceStateRecovering:  channelv1.DeviceState_DEVICE_STATE_RECOVERING,
	domain.DeviceStateQuarantined: channelv1.DeviceState_DEVICE_STATE_QUARANTINED,
}

var deviceStateFromProto = map[channelv1.DeviceState]domain.DeviceState{
	channelv1.DeviceState_DEVICE_STATE_ONLINE:      domain.DeviceStateOnline,
	channelv1.DeviceState_DEVICE_STATE_OFFLINE:     domain.DeviceStateOffline,
	channelv1.DeviceState_DEVICE_STATE_RECOVERING:  domain.DeviceStateRecovering,
	channelv1.DeviceState_DEVICE_STATE_QUARANTINED: domain.DeviceStateQuarantined,
}

var channelStateToProto = map[domain.ChannelState]channelv1.ChannelState{
	domain.ChannelUnresolved:   channelv1.ChannelState_CHANNEL_STATE_UNRESOLVED,
	domain.ChannelResolving:    channelv1.ChannelState_CHANNEL_STATE_UNRESOLVED,
	domain.ChannelOpening:      channelv1.ChannelState_CHANNEL_STATE_OPENING,
	domain.ChannelReady:        channelv1.ChannelState_CHANNEL_STATE_READY,
	domain.ChannelDegraded:     channelv1.ChannelState_CHANNEL_STATE_DEGRADED,
	domain.ChannelReconnecting: channelv1.ChannelState_CHANNEL_STATE_RECONNECTING,
	domain.ChannelClosed:       channelv1.ChannelState_CHANNEL_STATE_CLOSED,
	domain.ChannelFailed:       channelv1.ChannelState_CHANNEL_STATE_FAILED,
}

var channelStateFromProto = map[channelv1.ChannelState]domain.ChannelState{
	channelv1.ChannelState_CHANNEL_STATE_UNRESOLVED:   domain.ChannelUnresolved,
	channelv1.ChannelState_CHANNEL_STATE_OPENING:      domain.ChannelOpening,
	channelv1.ChannelState_CHANNEL_STATE_READY:        domain.ChannelReady,
	channelv1.ChannelState_CHANNEL_STATE_DEGRADED:     domain.ChannelDegraded,
	channelv1.ChannelState_CHANNEL_STATE_RECONNECTING: domain.ChannelReconnecting,
	channelv1.ChannelState_CHANNEL_STATE_CLOSED:       domain.ChannelClosed,
	channelv1.ChannelState_CHANNEL_STATE_FAILED:       domain.ChannelFailed,
}

var operationStateToProto = map[domain.OperationState]channelv1.OperationState{
	domain.OperationCreated:          channelv1.OperationState_OPERATION_STATE_CREATED,
	domain.OperationValidating:       channelv1.OperationState_OPERATION_STATE_VALIDATING,
	domain.OperationWaitingResource:  channelv1.OperationState_OPERATION_STATE_WAITING_RESOURCE,
	domain.OperationResolving:        channelv1.OperationState_OPERATION_STATE_RESOLVING,
	domain.OperationRunning:          channelv1.OperationState_OPERATION_STATE_RUNNING,
	domain.OperationVerifying:        channelv1.OperationState_OPERATION_STATE_VERIFYING,
	domain.OperationSucceeded:        channelv1.OperationState_OPERATION_STATE_SUCCEEDED,
	domain.OperationValidationFailed: channelv1.OperationState_OPERATION_STATE_VALIDATION_FAILED,
	domain.OperationResourceFailed:   channelv1.OperationState_OPERATION_STATE_RESOURCE_FAILED,
	domain.OperationResolveFailed:    channelv1.OperationState_OPERATION_STATE_RESOLVE_FAILED,
	domain.OperationRunFailed:        channelv1.OperationState_OPERATION_STATE_RUN_FAILED,
	domain.OperationVerifyFailed:     channelv1.OperationState_OPERATION_STATE_VERIFY_FAILED,
	domain.OperationCancelled:        channelv1.OperationState_OPERATION_STATE_CANCELLED,
	domain.OperationTimedOut:         channelv1.OperationState_OPERATION_STATE_TIMEOUT,
	domain.OperationUnknown:          channelv1.OperationState_OPERATION_STATE_UNKNOWN,
}

var operationStateFromProto = map[channelv1.OperationState]domain.OperationState{
	channelv1.OperationState_OPERATION_STATE_CREATED:           domain.OperationCreated,
	channelv1.OperationState_OPERATION_STATE_VALIDATING:        domain.OperationValidating,
	channelv1.OperationState_OPERATION_STATE_WAITING_RESOURCE:  domain.OperationWaitingResource,
	channelv1.OperationState_OPERATION_STATE_RESOLVING:         domain.OperationResolving,
	channelv1.OperationState_OPERATION_STATE_RUNNING:           domain.OperationRunning,
	channelv1.OperationState_OPERATION_STATE_VERIFYING:         domain.OperationVerifying,
	channelv1.OperationState_OPERATION_STATE_SUCCEEDED:         domain.OperationSucceeded,
	channelv1.OperationState_OPERATION_STATE_VALIDATION_FAILED: domain.OperationValidationFailed,
	channelv1.OperationState_OPERATION_STATE_RESOURCE_FAILED:   domain.OperationResourceFailed,
	channelv1.OperationState_OPERATION_STATE_RESOLVE_FAILED:    domain.OperationResolveFailed,
	channelv1.OperationState_OPERATION_STATE_RUN_FAILED:        domain.OperationRunFailed,
	channelv1.OperationState_OPERATION_STATE_VERIFY_FAILED:     domain.OperationVerifyFailed,
	channelv1.OperationState_OPERATION_STATE_CANCELLED:         domain.OperationCancelled,
	channelv1.OperationState_OPERATION_STATE_TIMEOUT:           domain.OperationTimedOut,
	channelv1.OperationState_OPERATION_STATE_UNKNOWN:           domain.OperationUnknown,
}

var sessionStateToProto = map[domain.SessionState]channelv1.SessionState{
	domain.SessionCreated:      channelv1.SessionState_SESSION_STATE_CREATED,
	domain.SessionActive:       channelv1.SessionState_SESSION_STATE_ACTIVE,
	domain.SessionDegraded:     channelv1.SessionState_SESSION_STATE_DEGRADED,
	domain.SessionReconnecting: channelv1.SessionState_SESSION_STATE_RECONNECTING,
	domain.SessionClosed:       channelv1.SessionState_SESSION_STATE_CLOSED,
	domain.SessionExpired:      channelv1.SessionState_SESSION_STATE_EXPIRED,
	domain.SessionRevoked:      channelv1.SessionState_SESSION_STATE_REVOKED,
	domain.SessionFailed:       channelv1.SessionState_SESSION_STATE_FAILED,
}

var sessionStateFromProto = map[channelv1.SessionState]domain.SessionState{
	channelv1.SessionState_SESSION_STATE_CREATED:      domain.SessionCreated,
	channelv1.SessionState_SESSION_STATE_ACTIVE:       domain.SessionActive,
	channelv1.SessionState_SESSION_STATE_DEGRADED:     domain.SessionDegraded,
	channelv1.SessionState_SESSION_STATE_RECONNECTING: domain.SessionReconnecting,
	channelv1.SessionState_SESSION_STATE_CLOSED:       domain.SessionClosed,
	channelv1.SessionState_SESSION_STATE_EXPIRED:      domain.SessionExpired,
	channelv1.SessionState_SESSION_STATE_REVOKED:      domain.SessionRevoked,
	channelv1.SessionState_SESSION_STATE_FAILED:       domain.SessionFailed,
}

var errorCategoryToProto = map[domain.ErrorCategory]channelv1.ErrorCategory{
	domain.CategoryValidation:    channelv1.ErrorCategory_ERROR_CATEGORY_VALIDATION,
	domain.CategoryAuthorization: channelv1.ErrorCategory_ERROR_CATEGORY_AUTHORIZATION,
	domain.CategoryResource:      channelv1.ErrorCategory_ERROR_CATEGORY_RESOURCE,
	domain.CategoryDiscovery:     channelv1.ErrorCategory_ERROR_CATEGORY_DISCOVERY,
	domain.CategoryConnection:    channelv1.ErrorCategory_ERROR_CATEGORY_CONNECTION,
	domain.CategoryProtocol:      channelv1.ErrorCategory_ERROR_CATEGORY_PROTOCOL,
	domain.CategoryDeviceState:   channelv1.ErrorCategory_ERROR_CATEGORY_DEVICE_STATE,
	domain.CategoryExecution:     channelv1.ErrorCategory_ERROR_CATEGORY_EXECUTION,
	domain.CategoryVerification:  channelv1.ErrorCategory_ERROR_CATEGORY_VERIFICATION,
	domain.CategoryTimeout:       channelv1.ErrorCategory_ERROR_CATEGORY_TIMEOUT,
	domain.CategoryCancellation:  channelv1.ErrorCategory_ERROR_CATEGORY_CANCELLATION,
	domain.CategoryInternal:      channelv1.ErrorCategory_ERROR_CATEGORY_INTERNAL,
	domain.CategoryUnknown:       channelv1.ErrorCategory_ERROR_CATEGORY_UNKNOWN,
}

var errorCategoryFromProto = map[channelv1.ErrorCategory]domain.ErrorCategory{
	channelv1.ErrorCategory_ERROR_CATEGORY_VALIDATION:    domain.CategoryValidation,
	channelv1.ErrorCategory_ERROR_CATEGORY_AUTHORIZATION: domain.CategoryAuthorization,
	channelv1.ErrorCategory_ERROR_CATEGORY_RESOURCE:      domain.CategoryResource,
	channelv1.ErrorCategory_ERROR_CATEGORY_DISCOVERY:     domain.CategoryDiscovery,
	channelv1.ErrorCategory_ERROR_CATEGORY_CONNECTION:    domain.CategoryConnection,
	channelv1.ErrorCategory_ERROR_CATEGORY_PROTOCOL:      domain.CategoryProtocol,
	channelv1.ErrorCategory_ERROR_CATEGORY_DEVICE_STATE:  domain.CategoryDeviceState,
	channelv1.ErrorCategory_ERROR_CATEGORY_EXECUTION:     domain.CategoryExecution,
	channelv1.ErrorCategory_ERROR_CATEGORY_VERIFICATION:  domain.CategoryVerification,
	channelv1.ErrorCategory_ERROR_CATEGORY_TIMEOUT:       domain.CategoryTimeout,
	channelv1.ErrorCategory_ERROR_CATEGORY_CANCELLATION:  domain.CategoryCancellation,
	channelv1.ErrorCategory_ERROR_CATEGORY_INTERNAL:      domain.CategoryInternal,
	channelv1.ErrorCategory_ERROR_CATEGORY_UNKNOWN:       domain.CategoryUnknown,
}

var riskLevelToProto = map[domain.RiskLevel]channelv1.RiskLevel{
	domain.RiskLow:      channelv1.RiskLevel_RISK_LEVEL_LOW,
	domain.RiskMedium:   channelv1.RiskLevel_RISK_LEVEL_MEDIUM,
	domain.RiskHigh:     channelv1.RiskLevel_RISK_LEVEL_HIGH,
	domain.RiskCritical: channelv1.RiskLevel_RISK_LEVEL_CRITICAL,
}

var riskLevelFromProto = map[channelv1.RiskLevel]domain.RiskLevel{
	channelv1.RiskLevel_RISK_LEVEL_LOW:      domain.RiskLow,
	channelv1.RiskLevel_RISK_LEVEL_MEDIUM:   domain.RiskMedium,
	channelv1.RiskLevel_RISK_LEVEL_HIGH:     domain.RiskHigh,
	channelv1.RiskLevel_RISK_LEVEL_CRITICAL: domain.RiskCritical,
}

var lockModeToProto = map[domain.LockMode]channelv1.LockMode{
	domain.LockShared:    channelv1.LockMode_LOCK_MODE_SHARED,
	domain.LockExclusive: channelv1.LockMode_LOCK_MODE_EXCLUSIVE,
}

var lockModeFromProto = map[channelv1.LockMode]domain.LockMode{
	channelv1.LockMode_LOCK_MODE_SHARED:    domain.LockShared,
	channelv1.LockMode_LOCK_MODE_EXCLUSIVE: domain.LockExclusive,
}

// ---------------------------------------------------------------------------
// Objects
// ---------------------------------------------------------------------------

func DeviceToProto(d *domain.Device) *channelv1.Device {
	if d == nil {
		return nil
	}
	p := &channelv1.Device{
		DeviceId:     string(d.ID),
		Serial:       d.Serial,
		Model:        d.Model,
		State:        deviceStateToProto[d.State],
		Properties:   d.Properties,
		ObservedAtMs: ms(d.ObservedAt),
	}
	for _, e := range d.Endpoints {
		p.EndpointIds = append(p.EndpointIds, string(e))
	}
	return p
}

func DeviceFromProto(p *channelv1.Device) *domain.Device {
	if p == nil {
		return nil
	}
	d := &domain.Device{
		ID:         domain.DeviceID(p.DeviceId),
		Serial:     p.Serial,
		Model:      p.Model,
		State:      deviceStateFromProto[p.State],
		Properties: p.Properties,
		ObservedAt: fromMs(p.ObservedAtMs),
	}
	for _, e := range p.EndpointIds {
		d.Endpoints = append(d.Endpoints, domain.EndpointID(e))
	}
	return d
}

func EndpointToProto(e *domain.Endpoint) *channelv1.Endpoint {
	if e == nil {
		return nil
	}
	return &channelv1.Endpoint{
		EndpointId:   string(e.ID),
		DeviceId:     string(e.DeviceID),
		EndpointType: string(e.Type),
		Locator:      e.Locator,
		Transport:    e.Transport,
		Attributes:   e.Attributes,
		Source:       e.Source,
	}
}

func EndpointFromProto(p *channelv1.Endpoint) *domain.Endpoint {
	if p == nil {
		return nil
	}
	return &domain.Endpoint{
		ID:         domain.EndpointID(p.EndpointId),
		DeviceID:   domain.DeviceID(p.DeviceId),
		Type:       domain.EndpointType(p.EndpointType),
		Locator:    p.Locator,
		Transport:  p.Transport,
		Attributes: p.Attributes,
		Source:     p.Source,
	}
}

func ChannelToProto(c *domain.Channel) *channelv1.ChannelDescriptor {
	if c == nil {
		return nil
	}
	p := &channelv1.ChannelDescriptor{
		ChannelId:   string(c.ID),
		PluginId:    c.PluginID,
		ChannelType: c.ChannelType,
		EndpointId:  string(c.EndpointID),
		DeviceId:    string(c.DeviceID),
		State:       channelStateToProto[c.State],
		Healthy:     c.Healthy,
		Cost:        c.Cost,
	}
	for _, cap := range c.Capabilities {
		p.Capabilities = append(p.Capabilities, string(cap))
	}
	return p
}

func ChannelFromProto(p *channelv1.ChannelDescriptor) *domain.Channel {
	if p == nil {
		return nil
	}
	c := &domain.Channel{
		ID:          domain.ChannelID(p.ChannelId),
		PluginID:    p.PluginId,
		ChannelType: p.ChannelType,
		EndpointID:  domain.EndpointID(p.EndpointId),
		DeviceID:    domain.DeviceID(p.DeviceId),
		State:       channelStateFromProto[p.State],
		Healthy:     p.Healthy,
		Cost:        p.Cost,
	}
	for _, cap := range p.Capabilities {
		c.Capabilities = append(c.Capabilities, domain.CapabilityName(cap))
	}
	return c
}

func CapabilityToProto(c *domain.Capability) *channelv1.Capability {
	if c == nil {
		return nil
	}
	p := &channelv1.Capability{
		Name:                  string(c.Name),
		Version:               c.Version,
		InputSchema:           c.InputSchema,
		OutputSchema:          c.OutputSchema,
		RiskLevel:             riskLevelToProto[c.RiskLevel],
		Idempotent:            c.Idempotent,
		SupportedChannelTypes: c.SupportedChannelTypes,
	}
	p.ResourceRequirements = c.ResourceRequirements
	return p
}

func CapabilityFromProto(p *channelv1.Capability) *domain.Capability {
	if p == nil {
		return nil
	}
	c := &domain.Capability{
		Name:                  domain.CapabilityName(p.Name),
		Version:               p.Version,
		InputSchema:           p.InputSchema,
		OutputSchema:          p.OutputSchema,
		RiskLevel:             riskLevelFromProto[p.RiskLevel],
		Idempotent:            p.Idempotent,
		SupportedChannelTypes: p.SupportedChannelTypes,
	}
	c.ResourceRequirements = p.ResourceRequirements
	return c
}

func OperationRequestToProto(r *domain.OperationRequest) *channelv1.OperationRequest {
	if r == nil {
		return nil
	}
	return &channelv1.OperationRequest{
		Capability:     string(r.Capability),
		Target:         string(r.Target),
		SessionId:      string(r.SessionID),
		Parameters:     r.Parameters,
		IdempotencyKey: r.IdempotencyKey,
		ChannelType:    r.ChannelType,
		DeadlineMs:     ms(r.Deadline),
	}
}

func OperationResultToProto(r *domain.OperationResult) *channelv1.OperationResult {
	if r == nil {
		return nil
	}
	p := &channelv1.OperationResult{
		OperationId:   string(r.OperationID),
		State:         operationStateToProto[r.State],
		Output:        r.Output,
		Error:         ErrorToProto(r.Error),
		StartedAtMs:   ms(r.StartedAt),
		CompletedAtMs: ms(r.CompletedAt),
	}
	for _, e := range r.EvidenceRefs {
		p.EvidenceRefs = append(p.EvidenceRefs, string(e))
	}
	for _, a := range r.ArtifactRefs {
		p.ArtifactRefs = append(p.ArtifactRefs, string(a))
	}
	return p
}

func ErrorToProto(e *domain.Error) *channelv1.Error {
	if e == nil {
		return nil
	}
	return &channelv1.Error{
		Code:        e.Code,
		Category:    errorCategoryToProto[e.Category],
		Message:     e.Message,
		Retryable:   e.Retryable,
		Recoverable: e.Recoverable,
		Severity:    e.Severity,
		Source:      e.Source,
		Details:     e.Details,
	}
}

func ErrorFromProto(p *channelv1.Error) *domain.Error {
	if p == nil {
		return nil
	}
	return &domain.Error{
		Code:        p.Code,
		Category:    errorCategoryFromProto[p.Category],
		Message:     p.Message,
		Retryable:   p.Retryable,
		Recoverable: p.Recoverable,
		Severity:    p.Severity,
		Source:      p.Source,
		Details:     p.Details,
	}
}

func SessionToProto(s *domain.Session) *channelv1.Session {
	if s == nil {
		return nil
	}
	return &channelv1.Session{
		SessionId:   string(s.ID),
		Principal:   s.Principal,
		DeviceId:    string(s.DeviceID),
		ChannelId:   string(s.ChannelID),
		State:       sessionStateToProto[s.State],
		ExpiresAtMs: ms(s.ExpiresAt),
		Permissions: s.Permissions,
	}
}

func SessionFromProto(p *channelv1.Session) *domain.Session {
	if p == nil {
		return nil
	}
	return &domain.Session{
		ID:          domain.SessionID(p.SessionId),
		Principal:   p.Principal,
		DeviceID:    domain.DeviceID(p.DeviceId),
		ChannelID:   domain.ChannelID(p.ChannelId),
		State:       sessionStateFromProto[p.State],
		ExpiresAt:   fromMs(p.ExpiresAtMs),
		Permissions: p.Permissions,
	}
}

func OperationResultFromProto(p *channelv1.OperationResult) *domain.OperationResult {
	if p == nil {
		return nil
	}
	r := &domain.OperationResult{
		OperationID: domain.OperationID(p.OperationId),
		State:       operationStateFromProto[p.State],
		Output:      p.Output,
		Error:       ErrorFromProto(p.Error),
		StartedAt:   fromMs(p.StartedAtMs),
		CompletedAt: fromMs(p.CompletedAtMs),
	}
	for _, e := range p.EvidenceRefs {
		r.EvidenceRefs = append(r.EvidenceRefs, domain.EvidenceID(e))
	}
	for _, a := range p.ArtifactRefs {
		r.ArtifactRefs = append(r.ArtifactRefs, domain.ArtifactID(a))
	}
	return r
}

func EventToProto(e *domain.Event) *channelv1.EventEnvelope {
	if e == nil {
		return nil
	}
	payload := ""
	if e.Payload != nil {
		if b, err := json.Marshal(e.Payload); err == nil {
			payload = string(b)
		}
	}
	return &channelv1.EventEnvelope{
		EventId:      string(e.ID),
		EventType:    e.Type,
		EventVersion: e.Version,
		Producer:     e.Producer,
		Subject:      e.Subject,
		Sequence:     int64(e.Sequence),
		OccurredAtMs: ms(e.OccurredAt),
		EmittedAtMs:  ms(e.EmittedAt),
		OperationId:  string(e.OperationID),
		Payload:      payload,
	}
}

func ArtifactToProto(a *domain.Artifact) *channelv1.Artifact {
	if a == nil {
		return nil
	}
	return &channelv1.Artifact{
		ArtifactId:  string(a.ID),
		Type:        a.Type,
		Uri:         a.URI,
		Checksum:    a.Checksum,
		SizeBytes:   a.SizeBytes,
		ContentType: a.ContentType,
		OperationId: string(a.OperationID),
	}
}

func EvidenceToProto(ev *domain.Evidence) *channelv1.Evidence {
	if ev == nil {
		return nil
	}
	return &channelv1.Evidence{
		EvidenceId:  string(ev.ID),
		Name:        ev.Name,
		Value:       ev.Value,
		ArtifactRef: string(ev.ArtifactRef),
		OperationId: string(ev.OperationID),
	}
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func fromMs(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v)
}

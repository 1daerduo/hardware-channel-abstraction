package sdk

import (
	"example.com/embedded-loop-channel/domain"
)

// Error builds a normalized domain.Error. Plugins must use this rather than
// leaking raw protocol errors (Design doc 12 §18).
func Error(code string, cat domain.ErrorCategory, msg string) *domain.Error {
	return domain.NewError(code, cat, msg)
}

// ProtocolError maps a raw protocol failure to the PROTOCOL category, keeping
// the raw text only as a detail.
func ProtocolError(code, msg, raw string) *domain.Error {
	return domain.NewError(code, domain.CategoryProtocol, msg).WithDetail("raw", raw)
}

// DeviceStateError maps a device-side condition (e.g. offline) to the
// DEVICE_STATE category.
func DeviceStateError(code, msg string) *domain.Error {
	return domain.NewError(code, domain.CategoryDeviceState, msg)
}

// ConnectionError maps transport loss to the CONNECTION category.
func ConnectionError(code, msg string) *domain.Error {
	return domain.NewError(code, domain.CategoryConnection, msg).WithDetail("recoverable", "true")
}

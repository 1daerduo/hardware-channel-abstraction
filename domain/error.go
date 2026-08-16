package domain

// ErrorCategory is the unified error taxonomy (Design doc 09). Automation
// must depend on the stable code/category, never on parsing message text.
type ErrorCategory string

const (
	CategoryValidation    ErrorCategory = "VALIDATION"
	CategoryAuthorization ErrorCategory = "AUTHORIZATION"
	CategoryResource      ErrorCategory = "RESOURCE"
	CategoryDiscovery     ErrorCategory = "DISCOVERY"
	CategoryConnection    ErrorCategory = "CONNECTION"
	CategoryProtocol      ErrorCategory = "PROTOCOL"
	CategoryDeviceState   ErrorCategory = "DEVICE_STATE"
	CategoryExecution     ErrorCategory = "EXECUTION"
	CategoryVerification  ErrorCategory = "VERIFICATION"
	CategoryTimeout       ErrorCategory = "TIMEOUT"
	CategoryCancellation  ErrorCategory = "CANCELLATION"
	CategoryInternal      ErrorCategory = "INTERNAL"
	CategoryUnknown       ErrorCategory = "UNKNOWN"
)

// Error is the NormalizedError contract. retryable, recoverable and
// re-executable are intentionally distinct: recoverable does NOT imply the
// action may be blindly re-executed.
type Error struct {
	Code        string
	Category    ErrorCategory
	Message     string
	Retryable   bool
	Recoverable bool
	Severity    string
	Source      string
	Details     map[string]string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return string(e.Category) + "/" + e.Code + ": " + e.Message
}

// NewError builds a normalized error with a filled details map.
func NewError(code string, cat ErrorCategory, msg string) *Error {
	return &Error{
		Code:     code,
		Category: cat,
		Message:  msg,
		Details:  map[string]string{},
	}
}

// WithDetail returns e after recording a detail (chainable).
func (e *Error) WithDetail(k, v string) *Error {
	if e.Details == nil {
		e.Details = map[string]string{}
	}
	e.Details[k] = v
	return e
}

// Common error codes, kept stable so consumers can switch on them.
const (
	CodePermissionDenied   = "PERMISSION_DENIED"
	CodeDeviceOffline      = "DEVICE_OFFLINE"
	CodeChannelLost        = "CHANNEL_LOST"
	CodeUnsupportedCap     = "UNSUPPORTED_CAPABILITY"
	CodeInvalidInput       = "INVALID_INPUT"
	CodeTimeout            = "TIMEOUT"
	CodeVerificationFailed = "VERIFICATION_FAILED"
	CodeUnknown            = "UNKNOWN"
	CodeInternal           = "INTERNAL"
	CodeNotCancellable     = "NOT_CANCELLABLE"
)

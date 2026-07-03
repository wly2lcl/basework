package llm

import (
	"errors"
	"testing"
)

func TestError_ImplementsErrorInterface(t *testing.T) {
	e := &Error{
		Type:          ErrorTypeRateLimit,
		Message:       "too many requests",
		StatusCode:    429,
		ProviderError: "rate limit exceeded",
	}
	var err error = e
	if err == nil {
		t.Fatal("Error should not be nil")
	}
}

func TestError_ErrorsAs(t *testing.T) {
	e := &Error{
		Type:          ErrorTypeAuth,
		Message:       "invalid API key",
		StatusCode:    401,
		ProviderError: "unauthorized",
	}

	var target *Error
	if !errors.As(e, &target) {
		t.Fatal("errors.As should extract *Error")
	}
	if target.Type != ErrorTypeAuth {
		t.Errorf("expected Type=%q, got %q", ErrorTypeAuth, target.Type)
	}
	if target.Message != "invalid API key" {
		t.Errorf("expected Message=%q, got %q", "invalid API key", target.Message)
	}
	if target.StatusCode != 401 {
		t.Errorf("expected StatusCode=401, got %d", target.StatusCode)
	}
}

func TestError_ErrorString(t *testing.T) {
	e := &Error{
		Type:          ErrorTypeContextOverflow,
		Message:       "context too long",
		StatusCode:    400,
		ProviderError: "maximum context length exceeded",
	}
	got := e.Error()
	want := "[context_overflow] context too long (status=400)"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestIsContextOverflow_True(t *testing.T) {
	e := &Error{Type: ErrorTypeContextOverflow}
	if !IsContextOverflow(e) {
		t.Error("IsContextOverflow should return true for ContextOverflow type")
	}
}

func TestIsContextOverflow_False(t *testing.T) {
	tests := []ErrorType{
		ErrorTypeRateLimit,
		ErrorTypeAuth,
		ErrorTypeNetwork,
		ErrorTypeModelNotFound,
		ErrorTypeInternal,
	}
	for _, typ := range tests {
		e := &Error{Type: typ}
		if IsContextOverflow(e) {
			t.Errorf("IsContextOverflow should return false for %q", typ)
		}
	}
}

func TestIsRateLimit_True(t *testing.T) {
	e := &Error{Type: ErrorTypeRateLimit}
	if !IsRateLimit(e) {
		t.Error("IsRateLimit should return true for RateLimit type")
	}
}

func TestIsRateLimit_False(t *testing.T) {
	tests := []ErrorType{
		ErrorTypeContextOverflow,
		ErrorTypeAuth,
		ErrorTypeNetwork,
		ErrorTypeModelNotFound,
		ErrorTypeInternal,
	}
	for _, typ := range tests {
		e := &Error{Type: typ}
		if IsRateLimit(e) {
			t.Errorf("IsRateLimit should return false for %q", typ)
		}
	}
}

func TestIsContextOverflow_FalseForNonMatchingWrappedError(t *testing.T) {
	base := &Error{Type: ErrorTypeRateLimit}
	if IsContextOverflow(base) {
		t.Error("IsContextOverflow should return false for RateLimit wrapped error")
	}
}

func TestIsRateLimit_FalseForNonMatchingWrappedError(t *testing.T) {
	base := &Error{Type: ErrorTypeContextOverflow}
	if IsRateLimit(base) {
		t.Error("IsRateLimit should return false for ContextOverflow wrapped error")
	}
}
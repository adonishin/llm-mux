package oauth

import (
	"strings"
	"testing"
)

func TestRenderSuccess(t *testing.T) {
	result, err := RenderSuccess()
	if err != nil {
		t.Fatalf("RenderSuccess() error = %v", err)
	}

	// Verify basic structure
	checks := []string{
		"<!DOCTYPE html>",
		"Authentication Successful",
		"icon-success",
		"window.close()",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("RenderSuccess() missing %q", check)
		}
	}
}

func TestRenderError(t *testing.T) {
	testMessage := "Test error message <script>alert('xss')</script>"
	result, err := RenderError(testMessage)
	if err != nil {
		t.Fatalf("RenderError() error = %v", err)
	}

	// Verify basic structure
	checks := []string{
		"<!DOCTYPE html>",
		"Authentication Failed",
		"icon-error",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("RenderError() missing %q", check)
		}
	}

	// Verify XSS escaping - script tag should be escaped
	if strings.Contains(result, "<script>alert") {
		t.Error("RenderError() did not escape script tag (XSS vulnerability)")
	}

	// Verify the escaped version is present
	if !strings.Contains(result, "Test error message") {
		t.Error("RenderError() missing error message content")
	}
}

func TestRenderSuccessWebUI(t *testing.T) {
	provider := "claude"
	state := "test-state-123"

	result, err := RenderSuccessWebUI(provider, state)
	if err != nil {
		t.Fatalf("RenderSuccessWebUI() error = %v", err)
	}

	// Verify postMessage integration
	checks := []string{
		"<!DOCTYPE html>",
		"oauth-callback",
		"postMessage",
		"window.opener",
		"window.parent",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("RenderSuccessWebUI() missing %q", check)
		}
	}
}

func TestRenderErrorWebUI(t *testing.T) {
	provider := "gemini"
	state := "test-state-456"
	message := "OAuth provider returned error"

	result, err := RenderErrorWebUI(provider, state, message)
	if err != nil {
		t.Fatalf("RenderErrorWebUI() error = %v", err)
	}

	// Verify postMessage integration
	checks := []string{
		"<!DOCTYPE html>",
		"oauth-callback",
		"postMessage",
		"status: 'error'",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("RenderErrorWebUI() missing %q", check)
		}
	}
}

func TestHTMLSuccessFunction(t *testing.T) {
	// Test the public API function
	result := HTMLSuccess()
	if result == "" {
		t.Error("HTMLSuccess() returned empty string")
	}
	if !strings.Contains(result, "<!DOCTYPE html>") {
		t.Error("HTMLSuccess() did not return valid HTML")
	}
}

func TestHTMLErrorFunction(t *testing.T) {
	result := HTMLError("Test error")
	if result == "" {
		t.Error("HTMLError() returned empty string")
	}
	if !strings.Contains(result, "<!DOCTYPE html>") {
		t.Error("HTMLError() did not return valid HTML")
	}
}

func TestHTMLSuccessWithPostMessageFunction(t *testing.T) {
	result := HTMLSuccessWithPostMessage("provider", "state")
	if result == "" {
		t.Error("HTMLSuccessWithPostMessage() returned empty string")
	}
	if !strings.Contains(result, "postMessage") {
		t.Error("HTMLSuccessWithPostMessage() missing postMessage")
	}
}

func TestHTMLErrorWithPostMessageFunction(t *testing.T) {
	result := HTMLErrorWithPostMessage("provider", "state", "error")
	if result == "" {
		t.Error("HTMLErrorWithPostMessage() returned empty string")
	}
	if !strings.Contains(result, "postMessage") {
		t.Error("HTMLErrorWithPostMessage() missing postMessage")
	}
}

// TestTemplateAccessibility verifies accessibility features
func TestTemplateAccessibility(t *testing.T) {
	result, err := RenderSuccess()
	if err != nil {
		t.Fatalf("RenderSuccess() error = %v", err)
	}

	accessibilityChecks := []string{
		`lang="en"`,           // Language attribute
		`role="main"`,         // ARIA landmark
		`aria-live="polite"`,  // Live region for screen readers
		`aria-hidden="true"`,  // Hidden decorative elements
	}

	for _, check := range accessibilityChecks {
		if !strings.Contains(result, check) {
			t.Errorf("RenderSuccess() missing accessibility attribute %q", check)
		}
	}
}

// TestTemplateDarkMode verifies dark mode support
func TestTemplateDarkMode(t *testing.T) {
	result, err := RenderSuccess()
	if err != nil {
		t.Fatalf("RenderSuccess() error = %v", err)
	}

	if !strings.Contains(result, "prefers-color-scheme: dark") {
		t.Error("RenderSuccess() missing dark mode support")
	}
}

// TestTemplateReducedMotion verifies reduced motion support
func TestTemplateReducedMotion(t *testing.T) {
	result, err := RenderSuccess()
	if err != nil {
		t.Fatalf("RenderSuccess() error = %v", err)
	}

	if !strings.Contains(result, "prefers-reduced-motion") {
		t.Error("RenderSuccess() missing reduced motion support")
	}
}

// TestTemplateResponsive verifies responsive design
func TestTemplateResponsive(t *testing.T) {
	result, err := RenderSuccess()
	if err != nil {
		t.Fatalf("RenderSuccess() error = %v", err)
	}

	if !strings.Contains(result, "viewport") {
		t.Error("RenderSuccess() missing viewport meta tag")
	}

	if !strings.Contains(result, "@media") {
		t.Error("RenderSuccess() missing media queries for responsive design")
	}
}

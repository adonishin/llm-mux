package managementasset

import (
	_ "embed"
)

//go:embed static/management.html
var EmbeddedManagementHTML []byte

// GetEmbeddedHTML returns the embedded management.html content.
func GetEmbeddedHTML() []byte {
	return EmbeddedManagementHTML
}

// HasEmbeddedHTML returns true if embedded HTML is available.
func HasEmbeddedHTML() bool {
	return len(EmbeddedManagementHTML) > 0
}

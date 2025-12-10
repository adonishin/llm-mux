// Package registry provides model family definitions for cross-provider routing.
// Model families allow clients to use a canonical model name (e.g., "claude-sonnet-4-5")
// and have it automatically routed to the appropriate provider-specific model ID.
package registry

// FamilyMember represents a provider-specific model within a family.
type FamilyMember struct {
	Provider string // Provider type (e.g., "kiro", "antigravity", "claude")
	ModelID  string // Provider-specific model ID
}

// ModelFamilies maps canonical model names to their provider-specific variants.
// Order matters: first available provider in the list will be used.
var ModelFamilies = map[string][]FamilyMember{
	// Claude Sonnet 4.5 family
	"claude-sonnet-4-5": {
		{Provider: "kiro", ModelID: "claude-sonnet-4-5"},
		{Provider: "antigravity", ModelID: "gemini-claude-sonnet-4-5"},
		{Provider: "claude", ModelID: "claude-sonnet-4-5-20250929"},
	},
	"claude-sonnet-4-5-thinking": {
		{Provider: "antigravity", ModelID: "gemini-claude-sonnet-4-5-thinking"},
		{Provider: "claude", ModelID: "claude-sonnet-4-5-thinking"},
	},

	// Claude Opus 4.5 family
	"claude-opus-4-5": {
		{Provider: "kiro", ModelID: "claude-opus-4-5-20251101"},
		{Provider: "claude", ModelID: "claude-opus-4-5-20251101"},
	},
	"claude-opus-4-5-thinking": {
		{Provider: "antigravity", ModelID: "gemini-claude-opus-4-5-thinking"},
		{Provider: "claude", ModelID: "claude-opus-4-5-thinking"},
	},

	// Claude Sonnet 4 family
	"claude-sonnet-4": {
		{Provider: "kiro", ModelID: "claude-sonnet-4-20250514"},
		{Provider: "claude", ModelID: "claude-sonnet-4-20250514"},
	},

	// Claude 3.7 Sonnet family
	"claude-3-7-sonnet": {
		{Provider: "kiro", ModelID: "claude-3-7-sonnet-20250219"},
		{Provider: "claude", ModelID: "claude-3-7-sonnet-20250219"},
	},

	// Gemini 2.5 Pro family - already consistent IDs
	"gemini-2.5-pro": {
		{Provider: "gemini-cli", ModelID: "gemini-2.5-pro"},
		{Provider: "antigravity", ModelID: "gemini-2.5-pro"},
		{Provider: "aistudio", ModelID: "gemini-2.5-pro"},
		{Provider: "gemini", ModelID: "gemini-2.5-pro"},
	},

	// Gemini 2.5 Flash family
	"gemini-2.5-flash": {
		{Provider: "gemini-cli", ModelID: "gemini-2.5-flash"},
		{Provider: "antigravity", ModelID: "gemini-2.5-flash"},
		{Provider: "aistudio", ModelID: "gemini-2.5-flash"},
		{Provider: "gemini", ModelID: "gemini-2.5-flash"},
	},

	// Gemini 2.5 Flash Lite family
	"gemini-2.5-flash-lite": {
		{Provider: "gemini-cli", ModelID: "gemini-2.5-flash-lite"},
		{Provider: "antigravity", ModelID: "gemini-2.5-flash-lite"},
		{Provider: "aistudio", ModelID: "gemini-2.5-flash-lite"},
		{Provider: "gemini", ModelID: "gemini-2.5-flash-lite"},
	},

	// Gemini 3 Pro Preview family
	"gemini-3-pro-preview": {
		{Provider: "gemini-cli", ModelID: "gemini-3-pro-preview"},
		{Provider: "antigravity", ModelID: "gemini-3-pro-preview"},
		{Provider: "aistudio", ModelID: "gemini-3-pro-preview"},
		{Provider: "gemini", ModelID: "gemini-3-pro-preview"},
	},

	// GPT-5.1 Codex Max family
	"gpt-5.1-codex-max": {
		{Provider: "github-copilot", ModelID: "gpt-5.1-codex-max"},
		{Provider: "openai", ModelID: "gpt-5.1-codex-max"},
	},
}

// ResolveModelFamily attempts to resolve a canonical model name to a provider-specific model.
// It checks available providers and returns the first matching provider and its model ID.
//
// Parameters:
//   - canonicalID: The canonical model name (e.g., "claude-sonnet-4-5")
//   - availableProviders: List of currently available provider types
//
// Returns:
//   - provider: The matched provider type
//   - modelID: The provider-specific model ID to use
//   - found: Whether a family match was found
func ResolveModelFamily(canonicalID string, availableProviders []string) (provider string, modelID string, found bool) {
	family, ok := ModelFamilies[canonicalID]
	if !ok {
		return "", canonicalID, false
	}

	// Create a set for O(1) lookup
	availableSet := make(map[string]bool, len(availableProviders))
	for _, p := range availableProviders {
		availableSet[p] = true
	}

	// Find first available provider in priority order
	for _, member := range family {
		if availableSet[member.Provider] {
			return member.Provider, member.ModelID, true
		}
	}

	return "", canonicalID, false
}

// GetCanonicalModelID returns the canonical ID for a provider-specific model ID.
// This is useful for reverse lookup (e.g., finding the family from a specific model).
//
// Returns empty string if no family contains this model ID.
func GetCanonicalModelID(providerModelID string) string {
	for canonical, members := range ModelFamilies {
		for _, member := range members {
			if member.ModelID == providerModelID {
				return canonical
			}
		}
	}
	return ""
}

// IsCanonicalID checks if the given ID is a canonical family name.
func IsCanonicalID(modelID string) bool {
	_, ok := ModelFamilies[modelID]
	return ok
}

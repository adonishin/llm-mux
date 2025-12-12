package management

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nghyane/llm-mux/internal/auth/claude"
	"github.com/nghyane/llm-mux/internal/auth/codex"
	"github.com/nghyane/llm-mux/internal/auth/copilot"
	"github.com/nghyane/llm-mux/internal/auth/qwen"
	"github.com/nghyane/llm-mux/internal/misc"
	"github.com/nghyane/llm-mux/internal/oauth"
	coreauth "github.com/nghyane/llm-mux/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// oauthService is the shared OAuth service instance for the unified API.
var oauthService = oauth.NewService()

// OAuthStartRequest represents the request body for starting an OAuth flow.
type OAuthStartRequest struct {
	Provider  string `json:"provider" binding:"required"`
	ProjectID string `json:"project_id,omitempty"`
}

// OAuthStartResponse represents the response for starting an OAuth flow.
type OAuthStartResponse struct {
	Status        string `json:"status"`
	AuthURL       string `json:"auth_url,omitempty"`
	State         string `json:"state,omitempty"`
	ID            string `json:"id,omitempty"`
	Error         string `json:"error,omitempty"`
	CodeVerifier  string `json:"code_verifier,omitempty"`  // For PKCE providers
	CodeChallenge string `json:"code_challenge,omitempty"` // For PKCE providers
	// Device flow fields
	FlowType        string `json:"flow_type,omitempty"`         // "oauth" or "device"
	UserCode        string `json:"user_code,omitempty"`         // Device flow user code
	VerificationURL string `json:"verification_url,omitempty"`  // Device flow verification URL
	ExpiresIn       int    `json:"expires_in,omitempty"`        // Device code expiry in seconds
	Interval        int    `json:"interval,omitempty"`          // Polling interval in seconds
}

// OAuthStart handles POST /v0/management/oauth/start
// Initiates an OAuth flow for the specified provider.
// Supports: OAuth (claude, codex, gemini, antigravity), Device Flow (qwen, copilot)
// Note: iFlow uses cookie-based auth - use POST /iflow-auth-url with {cookie: "..."} instead
func (h *Handler) OAuthStart(c *gin.Context) {
	var req OAuthStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, OAuthStartResponse{
			Status: "error",
			Error:  "Invalid request body: provider is required",
		})
		return
	}

	// Normalize provider name
	provider := req.Provider
	switch provider {
	case "claude", "anthropic":
		provider = "claude"
	case "gemini", "gemini-cli":
		provider = "gemini"
	case "copilot", "github-copilot":
		provider = "copilot"
	}

	// Check for device flow providers
	switch provider {
	case "qwen":
		h.startQwenDeviceFlow(c)
		return
	case "copilot":
		h.startCopilotDeviceFlow(c)
		return
	case "iflow":
		c.JSON(http.StatusBadRequest, OAuthStartResponse{
			Status: "error",
			Error:  "iFlow uses cookie-based auth. Use POST /v0/management/iflow-auth-url with {cookie: \"...\"} instead",
		})
		return
	}

	// Build auth URL for OAuth providers
	authURL, state, codeVerifier, err := h.buildProviderAuthURL(provider, req.ProjectID)
	if err != nil {
		c.JSON(http.StatusBadRequest, OAuthStartResponse{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	// Register the OAuth request in the service registry
	oauthService.Registry().Create(state, provider, oauth.ModeWebUI)

	// Start callback forwarder for WebUI mode
	targetURL, errTarget := h.managementCallbackURL("/" + provider + "/callback")
	if errTarget == nil {
		port := oauth.GetCallbackPort(provider)
		if port > 0 {
			_, _ = startCallbackForwarder(port, provider, targetURL)
		}
	}

	c.JSON(http.StatusOK, OAuthStartResponse{
		Status:       "ok",
		FlowType:     "oauth",
		AuthURL:      authURL,
		State:        state,
		ID:           state,
		CodeVerifier: codeVerifier,
	})
}

// Device flow timeout duration
const deviceFlowTimeout = 10 * time.Minute

// startQwenDeviceFlow initiates Qwen device authorization flow.
func (h *Handler) startQwenDeviceFlow(c *gin.Context) {
	// Use background context with timeout for device flow (not request context as it ends when response is sent)
	ctx, cancel := context.WithTimeout(context.Background(), deviceFlowTimeout)

	qwenAuth := qwen.NewQwenAuth(h.cfg)

	deviceFlow, err := qwenAuth.InitiateDeviceFlow(ctx)
	if err != nil {
		cancel()
		c.JSON(http.StatusInternalServerError, OAuthStartResponse{
			Status: "error",
			Error:  fmt.Sprintf("Failed to initiate device flow: %v", err),
		})
		return
	}

	state := fmt.Sprintf("qwen-%d", time.Now().UnixNano())

	// Register in registry for status tracking
	oauthService.Registry().Create(state, "qwen", oauth.ModeWebUI)

	// Start background goroutine to poll for token (pass cancel func for cleanup)
	go h.pollQwenToken(ctx, cancel, qwenAuth, deviceFlow, state)

	c.JSON(http.StatusOK, OAuthStartResponse{
		Status:          "ok",
		FlowType:        "device",
		State:           state,
		ID:              state,
		UserCode:        deviceFlow.UserCode,
		AuthURL:         deviceFlow.VerificationURIComplete,
		VerificationURL: deviceFlow.VerificationURI,
		ExpiresIn:       deviceFlow.ExpiresIn,
		Interval:        deviceFlow.Interval,
	})
}

// pollQwenToken polls for Qwen token in background and updates registry status.
func (h *Handler) pollQwenToken(ctx context.Context, cancel context.CancelFunc, qwenAuth *qwen.QwenAuth, deviceFlow *qwen.DeviceFlow, state string) {
	defer cancel() // Always cancel context when done to release resources

	log.WithField("state", state).Info("Waiting for Qwen authentication...")

	tokenData, err := qwenAuth.PollForToken(deviceFlow.DeviceCode, deviceFlow.CodeVerifier)
	if err != nil {
		// Check if cancelled/timed out
		if ctx.Err() != nil {
			oauthService.Registry().Cancel(state)
			log.WithField("state", state).Info("Qwen authentication cancelled or timed out")
			return
		}
		oauthService.Registry().Fail(state, fmt.Sprintf("Authentication failed: %v", err))
		log.WithError(err).WithField("state", state).Error("Qwen authentication failed")
		return
	}

	// Create token storage and save
	tokenStorage := qwenAuth.CreateTokenStorage(tokenData)
	tokenStorage.Email = fmt.Sprintf("qwen-%d", time.Now().UnixMilli())

	record := &coreauth.Auth{
		ID:       fmt.Sprintf("qwen-%s.json", tokenStorage.Email),
		Provider: "qwen",
		FileName: fmt.Sprintf("qwen-%s.json", tokenStorage.Email),
		Storage:  tokenStorage,
		Metadata: map[string]any{"email": tokenStorage.Email},
	}

	savedPath, errSave := h.saveTokenRecord(ctx, record)
	if errSave != nil {
		oauthService.Registry().Fail(state, fmt.Sprintf("Failed to save tokens: %v", errSave))
		log.WithError(errSave).WithField("state", state).Error("Failed to save Qwen tokens")
		return
	}

	// Mark as completed
	oauthService.Registry().Complete(state, &oauth.OAuthResult{
		State: state,
		Code:  "success",
	})

	log.WithFields(log.Fields{"state": state, "path": savedPath}).Info("Qwen authentication successful")
}

// startCopilotDeviceFlow initiates GitHub Copilot device authorization flow.
func (h *Handler) startCopilotDeviceFlow(c *gin.Context) {
	// Use background context with timeout for device flow
	ctx, cancel := context.WithTimeout(context.Background(), deviceFlowTimeout)

	copilotAuth := copilot.NewCopilotAuth(h.cfg)

	deviceCode, err := copilotAuth.StartDeviceFlow(ctx)
	if err != nil {
		cancel()
		c.JSON(http.StatusInternalServerError, OAuthStartResponse{
			Status: "error",
			Error:  fmt.Sprintf("Failed to start device flow: %v", err),
		})
		return
	}

	state := fmt.Sprintf("copilot-%s", deviceCode.DeviceCode[:8])

	// Register in registry for status tracking
	oauthService.Registry().Create(state, "copilot", oauth.ModeWebUI)

	// Start background goroutine to poll for token (pass cancel func for cleanup)
	go h.pollCopilotToken(ctx, cancel, copilotAuth, deviceCode, state)

	c.JSON(http.StatusOK, OAuthStartResponse{
		Status:          "ok",
		FlowType:        "device",
		State:           state,
		ID:              state,
		UserCode:        deviceCode.UserCode,
		AuthURL:         deviceCode.VerificationURI,
		VerificationURL: deviceCode.VerificationURI,
		ExpiresIn:       deviceCode.ExpiresIn,
		Interval:        deviceCode.Interval,
	})
}

// pollCopilotToken polls for GitHub Copilot token in background and updates registry status.
func (h *Handler) pollCopilotToken(ctx context.Context, cancel context.CancelFunc, copilotAuth *copilot.CopilotAuth, deviceCode *copilot.DeviceCodeResponse, state string) {
	defer cancel() // Always cancel context when done to release resources

	log.WithField("state", state).Info("Waiting for GitHub Copilot authentication...")

	creds, err := copilotAuth.WaitForAuthorization(ctx, deviceCode)
	if err != nil {
		// Check if cancelled/timed out
		if ctx.Err() != nil {
			oauthService.Registry().Cancel(state)
			log.WithField("state", state).Info("Copilot authentication cancelled or timed out")
			return
		}
		oauthService.Registry().Fail(state, fmt.Sprintf("Authentication failed: %v", err))
		log.WithError(err).WithField("state", state).Error("Copilot authentication failed")
		return
	}

	// Verify we can get a Copilot API token
	_, err = copilotAuth.GetCopilotAPIToken(ctx, creds.AccessToken)
	if err != nil {
		oauthService.Registry().Fail(state, fmt.Sprintf("Failed to verify Copilot access: %v", err))
		log.WithError(err).WithField("state", state).Error("Failed to verify Copilot access")
		return
	}

	// Build metadata and save
	metadata := map[string]any{
		"type":         "github-copilot",
		"access_token": creds.AccessToken,
		"token_type":   creds.TokenType,
		"scope":        creds.Scope,
		"username":     creds.Username,
		"timestamp":    time.Now().UnixMilli(),
	}

	fileName := fmt.Sprintf("github-copilot-%s.json", creds.Username)
	record := &coreauth.Auth{
		ID:       fileName,
		Provider: "github-copilot",
		FileName: fileName,
		Label:    creds.Username,
		Metadata: metadata,
	}

	savedPath, errSave := h.saveTokenRecord(ctx, record)
	if errSave != nil {
		oauthService.Registry().Fail(state, fmt.Sprintf("Failed to save tokens: %v", errSave))
		log.WithError(errSave).WithField("state", state).Error("Failed to save Copilot tokens")
		return
	}

	// Mark as completed
	oauthService.Registry().Complete(state, &oauth.OAuthResult{
		State: state,
		Code:  "success",
	})

	log.WithFields(log.Fields{"state": state, "path": savedPath, "user": creds.Username}).Info("GitHub Copilot authentication successful")
}

// OAuthStatus handles GET /v0/management/oauth/status/:state
// Returns the current status of an OAuth request.
func (h *Handler) OAuthStatus(c *gin.Context) {
	state := c.Param("state")
	if state == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "state parameter is required",
		})
		return
	}

	resp, err := oauthService.GetStatus(state)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "OAuth state not found or expired",
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// OAuthCancel handles POST /v0/management/oauth/cancel/:state
// Cancels a pending OAuth request.
func (h *Handler) OAuthCancel(c *gin.Context) {
	state := c.Param("state")
	if state == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "state parameter is required",
		})
		return
	}

	if err := oauthService.Cancel(state); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "OAuth state not found or already completed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// buildProviderAuthURL builds the authorization URL for a provider.
// Returns: authURL, state, codeVerifier (for PKCE), error
// Note: Only OAuth providers are supported here. Device flow providers (qwen, copilot)
// and cookie-based auth (iflow) are handled separately.
func (h *Handler) buildProviderAuthURL(provider, projectID string) (string, string, string, error) {
	state, err := misc.GenerateRandomState()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate state: %w", err)
	}

	switch provider {
	case "claude", "anthropic":
		return h.buildClaudeAuthURL(state)
	case "codex":
		return h.buildCodexAuthURL(state)
	case "gemini", "gemini-cli":
		return h.buildGeminiAuthURL(state)
	case "antigravity":
		return h.buildAntigravityAuthURL(state)
	default:
		return "", "", "", fmt.Errorf("unsupported OAuth provider: %s. Use device flow for qwen/copilot, or cookie auth for iflow", provider)
	}
}

// buildClaudeAuthURL builds the authorization URL for Claude/Anthropic.
func (h *Handler) buildClaudeAuthURL(state string) (string, string, string, error) {
	pkceCodes, err := claude.GeneratePKCECodes()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate PKCE codes: %w", err)
	}

	anthropicAuth := claude.NewClaudeAuth(h.cfg)
	authURL, _, err := anthropicAuth.GenerateAuthURL(state, pkceCodes)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate auth URL: %w", err)
	}

	return authURL, state, pkceCodes.CodeVerifier, nil
}

// buildCodexAuthURL builds the authorization URL for OpenAI Codex.
func (h *Handler) buildCodexAuthURL(state string) (string, string, string, error) {
	pkceCodes, err := codex.GeneratePKCECodes()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate PKCE codes: %w", err)
	}

	codexAuth := codex.NewCodexAuth(h.cfg)
	authURL, err := codexAuth.GenerateAuthURL(state, pkceCodes)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate auth URL: %w", err)
	}

	return authURL, state, pkceCodes.CodeVerifier, nil
}

// buildGeminiAuthURL builds the authorization URL for Gemini CLI.
func (h *Handler) buildGeminiAuthURL(state string) (string, string, string, error) {
	redirectURI := fmt.Sprintf("http://localhost:%d/oauth2callback", oauth.GetCallbackPort("gemini"))

	conf := &oauth2.Config{
		ClientID:     oauth.GeminiClientID,
		ClientSecret: oauth.GeminiClientSecret,
		RedirectURL:  redirectURI,
		Scopes:       []string{"openid", "https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/cloud-platform"},
		Endpoint:     google.Endpoint,
	}

	authURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	return authURL, state, "", nil
}

// buildAntigravityAuthURL builds the authorization URL for Antigravity/Google Cloud Code.
func (h *Handler) buildAntigravityAuthURL(state string) (string, string, string, error) {
	redirectURI := fmt.Sprintf("http://localhost:%d/oauth-callback", oauth.GetCallbackPort("antigravity"))

	conf := &oauth2.Config{
		ClientID:     oauth.AntigravityClientID,
		ClientSecret: oauth.AntigravityClientSecret,
		RedirectURL:  redirectURI,
		Scopes:       []string{"openid", "https://www.googleapis.com/auth/userinfo.email"},
		Endpoint:     google.Endpoint,
	}

	authURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	return authURL, state, "", nil
}

// GetOAuthService returns the shared OAuth service instance.
func GetOAuthService() *oauth.Service {
	return oauthService
}

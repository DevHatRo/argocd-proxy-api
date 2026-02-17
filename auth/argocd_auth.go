package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"argocd-proxy/config"
	"argocd-proxy/metrics"
	"argocd-proxy/types"
)

// TokenCache represents cached token information
type TokenCache struct {
	Token     string
	ExpiresAt time.Time
	IssuedAt  time.Time
}

// AuthService manages ArgoCD authentication and token caching
type AuthService struct {
	config          *config.Config
	httpClient      *http.Client
	tokenCache      *TokenCache
	refreshMutex    sync.Mutex
	refreshingToken bool
}

// NewAuthService creates a new authentication service
func NewAuthService(cfg *config.Config) *AuthService {
	return &AuthService{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetValidToken returns a valid ArgoCD token, refreshing if necessary
func (a *AuthService) GetValidToken(ctx context.Context) (string, error) {
	a.refreshMutex.Lock()
	defer a.refreshMutex.Unlock()

	// Check if we have a valid cached token
	if a.tokenCache != nil && a.isTokenValid() {
		return a.tokenCache.Token, nil
	}

	// If already refreshing, wait for completion
	if a.refreshingToken {
		// Release lock temporarily to allow other operations
		a.refreshMutex.Unlock()
		for a.refreshingToken {
			time.Sleep(100 * time.Millisecond)
		}
		a.refreshMutex.Lock()

		// Check again after waiting
		if a.tokenCache != nil && a.isTokenValid() {
			return a.tokenCache.Token, nil
		}
	}

	// Refresh the token
	return a.refreshToken(ctx)
}

// isTokenValid checks if the current token is valid and not expiring soon
func (a *AuthService) isTokenValid() bool {
	if a.tokenCache == nil {
		return false
	}

	// Refresh token 5 minutes before expiration
	refreshTime := a.tokenCache.ExpiresAt.Add(-5 * time.Minute)
	return time.Now().Before(refreshTime)
}

// refreshToken obtains a new token from ArgoCD
func (a *AuthService) refreshToken(ctx context.Context) (string, error) {
	a.refreshingToken = true
	defer func() {
		a.refreshingToken = false
	}()

	start := time.Now()
	log.Println("Refreshing ArgoCD token...")

	recordResult := func(result string) {
		metrics.TokenRefreshDuration.Observe(time.Since(start).Seconds())
		metrics.TokenRefreshTotal.WithLabelValues(result).Inc()
	}

	// Prepare the session request
	sessionReq := types.ArgocdSessionRequest{
		Username: a.config.ArgocdUsername,
		Password: a.config.ArgocdPassword,
	}

	reqBody, err := json.Marshal(sessionReq)
	if err != nil {
		recordResult("failure")
		return "", fmt.Errorf("failed to marshal session request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/session", a.config.ArgocdAPIURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		recordResult("failure")
		return "", fmt.Errorf("failed to create session request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		recordResult("failure")
		return "", fmt.Errorf("failed to execute session request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		recordResult("failure")
		return "", fmt.Errorf("ArgoCD authentication failed with status %d", resp.StatusCode)
	}

	// Parse the response
	var sessionResp types.ArgocdSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		recordResult("failure")
		return "", fmt.Errorf("failed to decode session response: %w", err)
	}

	if sessionResp.Token == "" {
		recordResult("failure")
		return "", fmt.Errorf("received empty token from ArgoCD")
	}

	// Cache the new token
	// ArgoCD tokens typically expire after 24 hours, but we'll use a conservative 23 hours
	now := time.Now()
	a.tokenCache = &TokenCache{
		Token:     sessionResp.Token,
		ExpiresAt: now.Add(23 * time.Hour),
		IssuedAt:  now,
	}

	recordResult("success")
	log.Printf("Successfully refreshed ArgoCD token, expires at: %s", a.tokenCache.ExpiresAt.Format(time.RFC3339))
	return a.tokenCache.Token, nil
}

// GetTokenStatus returns information about the current token status
func (a *AuthService) GetTokenStatus() map[string]interface{} {
	a.refreshMutex.Lock()
	defer a.refreshMutex.Unlock()

	status := map[string]interface{}{
		"hasToken": false,
		"isValid":  false,
	}

	if a.tokenCache != nil {
		status["hasToken"] = true
		status["isValid"] = a.isTokenValid()
		status["issuedAt"] = a.tokenCache.IssuedAt.Format(time.RFC3339)
		status["expiresAt"] = a.tokenCache.ExpiresAt.Format(time.RFC3339)

		// Calculate time until expiration
		timeUntilExpiry := time.Until(a.tokenCache.ExpiresAt)
		status["timeUntilExpiry"] = timeUntilExpiry.String()

		// Check if token is expiring soon (within 5 minutes)
		status["expiringSoon"] = timeUntilExpiry < 5*time.Minute
	}

	return status
}

// InvalidateToken invalidates the current cached token
func (a *AuthService) InvalidateToken() {
	a.refreshMutex.Lock()
	defer a.refreshMutex.Unlock()

	log.Println("Invalidating cached ArgoCD token")
	a.tokenCache = nil
}

// StartTokenRefreshRoutine starts a background routine to automatically refresh tokens
func (a *AuthService) StartTokenRefreshRoutine(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute) // Check every minute
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("Stopping token refresh routine")
				return
			case <-ticker.C:
				// Try to get a valid token, which will trigger refresh if needed
				if _, err := a.GetValidToken(ctx); err != nil {
					log.Printf("Failed to refresh token in background routine: %v", err)
				}
			}
		}
	}()
}

// CreateAuthenticatedRequest creates an HTTP request with ArgoCD authentication
func (a *AuthService) CreateAuthenticatedRequest(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	token, err := a.GetValidToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get valid token: %w", err)
	}

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewBuffer(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add ArgoCD authentication header
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"argocd-proxy/config"
	"argocd-proxy/types"
)

func TestNewAuthService(t *testing.T) {
	cfg := &config.Config{
		ArgocdAPIURL:   "https://argocd.example.com/api/v1",
		ArgocdUsername: "testuser",
		ArgocdPassword: "testpass",
	}

	authService := NewAuthService(cfg)

	if authService.config != cfg {
		t.Errorf("NewAuthService() config not set correctly")
	}
	if authService.httpClient == nil {
		t.Errorf("NewAuthService() httpClient not initialized")
	}
	if authService.httpClient.Timeout != 10*time.Second {
		t.Errorf("NewAuthService() httpClient timeout = %v, want %v", authService.httpClient.Timeout, 10*time.Second)
	}
}

func TestIsTokenValid(t *testing.T) {
	authService := &AuthService{}

	tests := []struct {
		name        string
		tokenCache  *TokenCache
		currentTime time.Time
		expected    bool
	}{
		{
			name:        "no token cache",
			tokenCache:  nil,
			currentTime: time.Now(),
			expected:    false,
		},
		{
			name: "token valid (not expiring soon)",
			tokenCache: &TokenCache{
				Token:     "valid-token",
				ExpiresAt: time.Now().Add(10 * time.Minute),
				IssuedAt:  time.Now().Add(-1 * time.Hour),
			},
			expected: true,
		},
		{
			name: "token expiring soon (within 5 minutes)",
			tokenCache: &TokenCache{
				Token:     "expiring-token",
				ExpiresAt: time.Now().Add(3 * time.Minute),
				IssuedAt:  time.Now().Add(-1 * time.Hour),
			},
			expected: false,
		},
		{
			name: "token expired",
			tokenCache: &TokenCache{
				Token:     "expired-token",
				ExpiresAt: time.Now().Add(-1 * time.Minute),
				IssuedAt:  time.Now().Add(-2 * time.Hour),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authService.tokenCache = tt.tokenCache
			result := authService.isTokenValid()
			if result != tt.expected {
				t.Errorf("isTokenValid() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRefreshToken(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectError    bool
		expectedToken  string
	}{
		{
			name: "successful token refresh",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
				}
				if r.URL.Path != "/session" {
					t.Errorf("Expected /session path, got %s", r.URL.Path)
				}

				// Verify request body
				var sessionReq types.ArgocdSessionRequest
				if err := json.NewDecoder(r.Body).Decode(&sessionReq); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}
				if sessionReq.Username != "testuser" || sessionReq.Password != "testpass" {
					t.Errorf("Invalid credentials in request")
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(types.ArgocdSessionResponse{
					Token: "new-token-123",
				})
			},
			expectError:   false,
			expectedToken: "new-token-123",
		},
		{
			name: "authentication failure",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("Unauthorized"))
			},
			expectError: true,
		},
		{
			name: "empty token response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(types.ArgocdSessionResponse{
					Token: "",
				})
			},
			expectError: true,
		},
		{
			name: "invalid JSON response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("invalid json"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := &config.Config{
				ArgocdAPIURL:   server.URL,
				ArgocdUsername: "testuser",
				ArgocdPassword: "testpass",
			}

			authService := NewAuthService(cfg)
			ctx := context.Background()

			token, err := authService.refreshToken(ctx)

			if tt.expectError {
				if err == nil {
					t.Errorf("refreshToken() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("refreshToken() unexpected error: %v", err)
				return
			}

			if token != tt.expectedToken {
				t.Errorf("refreshToken() token = %v, want %v", token, tt.expectedToken)
			}

			// Verify token is cached
			if authService.tokenCache == nil {
				t.Errorf("refreshToken() did not cache token")
			} else if authService.tokenCache.Token != tt.expectedToken {
				t.Errorf("refreshToken() cached token = %v, want %v", authService.tokenCache.Token, tt.expectedToken)
			}
		})
	}
}

func TestGetValidToken(t *testing.T) {
	tests := []struct {
		name              string
		initialTokenCache *TokenCache
		serverResponse    func(w http.ResponseWriter, r *http.Request)
		expectError       bool
		expectedToken     string
	}{
		{
			name: "use cached valid token",
			initialTokenCache: &TokenCache{
				Token:     "cached-token",
				ExpiresAt: time.Now().Add(10 * time.Minute),
				IssuedAt:  time.Now().Add(-1 * time.Hour),
			},
			expectedToken: "cached-token",
		},
		{
			name: "refresh expired token",
			initialTokenCache: &TokenCache{
				Token:     "expired-token",
				ExpiresAt: time.Now().Add(-1 * time.Minute),
				IssuedAt:  time.Now().Add(-2 * time.Hour),
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(types.ArgocdSessionResponse{
					Token: "refreshed-token",
				})
			},
			expectedToken: "refreshed-token",
		},
		{
			name: "no cached token",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(types.ArgocdSessionResponse{
					Token: "new-token",
				})
			},
			expectedToken: "new-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.serverResponse != nil {
				server = httptest.NewServer(http.HandlerFunc(tt.serverResponse))
				defer server.Close()
			}

			cfg := &config.Config{
				ArgocdUsername: "testuser",
				ArgocdPassword: "testpass",
			}
			if server != nil {
				cfg.ArgocdAPIURL = server.URL
			}

			authService := NewAuthService(cfg)
			authService.tokenCache = tt.initialTokenCache

			ctx := context.Background()
			token, err := authService.GetValidToken(ctx)

			if tt.expectError {
				if err == nil {
					t.Errorf("GetValidToken() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("GetValidToken() unexpected error: %v", err)
				return
			}

			if token != tt.expectedToken {
				t.Errorf("GetValidToken() token = %v, want %v", token, tt.expectedToken)
			}
		})
	}
}

func TestConcurrentTokenRefresh(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Add a small delay to increase chance of concurrent requests
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.ArgocdSessionResponse{
			Token: "concurrent-token",
		})
	}))
	defer server.Close()

	cfg := &config.Config{
		ArgocdAPIURL:   server.URL,
		ArgocdUsername: "testuser",
		ArgocdPassword: "testpass",
	}

	authService := NewAuthService(cfg)
	ctx := context.Background()

	// Launch multiple concurrent token requests
	const numConcurrent = 10
	tokens := make([]string, numConcurrent)
	errors := make([]error, numConcurrent)
	var wg sync.WaitGroup

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			token, err := authService.GetValidToken(ctx)
			tokens[index] = token
			errors[index] = err
		}(i)
	}

	wg.Wait()

	// Verify all requests succeeded
	for i, err := range errors {
		if err != nil {
			t.Errorf("Concurrent request %d failed: %v", i, err)
		}
	}

	// Verify all tokens are the same (from the same refresh)
	expectedToken := "concurrent-token"
	for i, token := range tokens {
		if token != expectedToken {
			t.Errorf("Concurrent request %d token = %v, want %v", i, token, expectedToken)
		}
	}

	// Verify only one or very few refresh requests were made
	// (Due to the concurrent safety mechanism)
	if requestCount > 3 { // Allow some tolerance for timing
		t.Errorf("Too many refresh requests made: %d, expected 1-3", requestCount)
	}
}

func TestGetTokenStatus(t *testing.T) {
	tests := []struct {
		name          string
		tokenCache    *TokenCache
		expectedKeys  []string
		expectedBools map[string]bool
	}{
		{
			name:         "no token",
			tokenCache:   nil,
			expectedKeys: []string{"hasToken", "isValid"},
			expectedBools: map[string]bool{
				"hasToken": false,
				"isValid":  false,
			},
		},
		{
			name: "valid token",
			tokenCache: &TokenCache{
				Token:     "valid-token",
				ExpiresAt: time.Now().Add(10 * time.Minute),
				IssuedAt:  time.Now().Add(-1 * time.Hour),
			},
			expectedKeys: []string{"hasToken", "isValid", "issuedAt", "expiresAt", "timeUntilExpiry", "expiringSoon"},
			expectedBools: map[string]bool{
				"hasToken":     true,
				"isValid":      true,
				"expiringSoon": false,
			},
		},
		{
			name: "expiring token",
			tokenCache: &TokenCache{
				Token:     "expiring-token",
				ExpiresAt: time.Now().Add(2 * time.Minute),
				IssuedAt:  time.Now().Add(-1 * time.Hour),
			},
			expectedKeys: []string{"hasToken", "isValid", "issuedAt", "expiresAt", "timeUntilExpiry", "expiringSoon"},
			expectedBools: map[string]bool{
				"hasToken":     true,
				"isValid":      false, // Should be false as it's expiring soon
				"expiringSoon": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authService := &AuthService{}
			authService.tokenCache = tt.tokenCache

			status := authService.GetTokenStatus()

			// Check expected keys exist
			for _, key := range tt.expectedKeys {
				if _, exists := status[key]; !exists {
					t.Errorf("GetTokenStatus() missing key %s", key)
				}
			}

			// Check expected boolean values
			for key, expectedBool := range tt.expectedBools {
				if actualBool, ok := status[key].(bool); !ok {
					t.Errorf("GetTokenStatus() key %s is not a boolean", key)
				} else if actualBool != expectedBool {
					t.Errorf("GetTokenStatus() key %s = %v, want %v", key, actualBool, expectedBool)
				}
			}
		})
	}
}

func TestInvalidateToken(t *testing.T) {
	authService := &AuthService{
		tokenCache: &TokenCache{
			Token:     "cached-token",
			ExpiresAt: time.Now().Add(10 * time.Minute),
			IssuedAt:  time.Now().Add(-1 * time.Hour),
		},
	}

	// Verify token exists
	if authService.tokenCache == nil {
		t.Errorf("Setup failed: token cache should exist")
	}

	authService.InvalidateToken()

	// Verify token is invalidated
	if authService.tokenCache != nil {
		t.Errorf("InvalidateToken() did not clear token cache")
	}
}

func TestCreateAuthenticatedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.ArgocdSessionResponse{
			Token: "test-token-123",
		})
	}))
	defer server.Close()

	cfg := &config.Config{
		ArgocdAPIURL:   server.URL,
		ArgocdUsername: "testuser",
		ArgocdPassword: "testpass",
	}

	authService := NewAuthService(cfg)
	ctx := context.Background()

	tests := []struct {
		name           string
		method         string
		url            string
		body           []byte
		expectedMethod string
		expectedAuth   string
	}{
		{
			name:           "GET request",
			method:         "GET",
			url:            "https://api.example.com/projects",
			body:           nil,
			expectedMethod: "GET",
			expectedAuth:   "Bearer test-token-123",
		},
		{
			name:           "POST request with body",
			method:         "POST",
			url:            "https://api.example.com/applications",
			body:           []byte(`{"name":"test-app"}`),
			expectedMethod: "POST",
			expectedAuth:   "Bearer test-token-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := authService.CreateAuthenticatedRequest(ctx, tt.method, tt.url, tt.body)
			if err != nil {
				t.Errorf("CreateAuthenticatedRequest() error = %v", err)
				return
			}

			if req.Method != tt.expectedMethod {
				t.Errorf("CreateAuthenticatedRequest() method = %v, want %v", req.Method, tt.expectedMethod)
			}

			if req.URL.String() != tt.url {
				t.Errorf("CreateAuthenticatedRequest() URL = %v, want %v", req.URL.String(), tt.url)
			}

			authHeader := req.Header.Get("Authorization")
			if authHeader != tt.expectedAuth {
				t.Errorf("CreateAuthenticatedRequest() Authorization header = %v, want %v", authHeader, tt.expectedAuth)
			}

			contentType := req.Header.Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("CreateAuthenticatedRequest() Content-Type header = %v, want %v", contentType, "application/json")
			}
		})
	}
}

func TestStartTokenRefreshRoutine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.ArgocdSessionResponse{
			Token: "routine-token",
		})
	}))
	defer server.Close()

	cfg := &config.Config{
		ArgocdAPIURL:   server.URL,
		ArgocdUsername: "testuser",
		ArgocdPassword: "testpass",
	}

	authService := NewAuthService(cfg)

	// Set an expiring token to trigger refresh
	authService.tokenCache = &TokenCache{
		Token:     "expiring-token",
		ExpiresAt: time.Now().Add(2 * time.Minute), // Will trigger refresh
		IssuedAt:  time.Now().Add(-1 * time.Hour),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start the refresh routine
	authService.StartTokenRefreshRoutine(ctx)

	// Wait a bit for the routine to potentially run
	time.Sleep(500 * time.Millisecond)

	// The routine should have refreshed the token
	status := authService.GetTokenStatus()
	if hasToken, ok := status["hasToken"].(bool); !ok || !hasToken {
		t.Errorf("Token refresh routine should have maintained a token")
	}
}

// Benchmark tests for performance-critical functions
func BenchmarkGetValidToken(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.ArgocdSessionResponse{
			Token: "benchmark-token",
		})
	}))
	defer server.Close()

	cfg := &config.Config{
		ArgocdAPIURL:   server.URL,
		ArgocdUsername: "testuser",
		ArgocdPassword: "testpass",
	}

	authService := NewAuthService(cfg)

	// Set a valid token to avoid HTTP calls during benchmark
	authService.tokenCache = &TokenCache{
		Token:     "cached-token",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IssuedAt:  time.Now().Add(-1 * time.Hour),
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		authService.GetValidToken(ctx)
	}
}

func BenchmarkGetTokenStatus(b *testing.B) {
	authService := &AuthService{
		tokenCache: &TokenCache{
			Token:     "cached-token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
			IssuedAt:  time.Now().Add(-1 * time.Hour),
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		authService.GetTokenStatus()
	}
}

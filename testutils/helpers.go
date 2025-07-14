package testutils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"argocd-proxy/config"
	"argocd-proxy/types"
)

// SetupTestEnv sets up environment variables for testing and returns a cleanup function
func SetupTestEnv(envVars map[string]string) func() {
	// Store original values
	originalVars := make(map[string]string)
	for key := range envVars {
		originalVars[key] = os.Getenv(key)
	}

	// Set test values
	for key, value := range envVars {
		os.Setenv(key, value)
	}

	// Return cleanup function
	return func() {
		for key, originalValue := range originalVars {
			if originalValue == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, originalValue)
			}
		}
	}
}

// CreateTestConfig creates a test configuration with default values
func CreateTestConfig() *config.Config {
	return &config.Config{
		Port:           "5001",
		ArgocdAPIURL:   "https://test-argocd.example.com/api/v1",
		ArgocdUsername: "testuser",
		ArgocdPassword: "testpass",
		ProjectGroups: []config.ProjectGroup{
			{
				Name:        "Frontend",
				Description: "Frontend applications",
				Projects:    []string{"web-app", "mobile-app"},
			},
			{
				Name:        "Backend",
				Description: "Backend services",
				Projects:    []string{"api-service", "auth-service"},
			},
		},
		IgnoredProjects: []string{"test-*", "*-dev", "temp-*"},
	}
}

// CreateTestProjects returns a slice of test ArgoCD projects
func CreateTestProjects() []types.ArgocdProject {
	return []types.ArgocdProject{
		{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "AppProject",
			Metadata: types.ArgocdProjectMetadata{
				Name:      "production",
				Namespace: "argocd",
				Labels: map[string]string{
					"env": "production",
				},
			},
			Spec: types.ArgocdProjectSpec{
				Description: "Production applications",
				SourceRepos: []string{"https://github.com/company/apps"},
				Destinations: []types.ArgocdProjectDestination{
					{
						Server:    "https://kubernetes.default.svc",
						Namespace: "production",
					},
				},
			},
		},
		{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "AppProject",
			Metadata: types.ArgocdProjectMetadata{
				Name:      "staging",
				Namespace: "argocd",
				Labels: map[string]string{
					"env": "staging",
				},
			},
			Spec: types.ArgocdProjectSpec{
				Description: "Staging applications",
				SourceRepos: []string{"https://github.com/company/apps"},
				Destinations: []types.ArgocdProjectDestination{
					{
						Server:    "https://kubernetes.default.svc",
						Namespace: "staging",
					},
				},
			},
		},
		{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "AppProject",
			Metadata: types.ArgocdProjectMetadata{
				Name:      "test-project",
				Namespace: "argocd",
				Labels: map[string]string{
					"env": "test",
				},
			},
			Spec: types.ArgocdProjectSpec{
				Description: "Test applications - should be filtered",
				SourceRepos: []string{"https://github.com/company/test-apps"},
				Destinations: []types.ArgocdProjectDestination{
					{
						Server:    "https://kubernetes.default.svc",
						Namespace: "test",
					},
				},
			},
		},
	}
}

// CreateTestApplications returns a slice of test ArgoCD applications
func CreateTestApplications() []types.ArgocdApplication {
	return []types.ArgocdApplication{
		{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "Application",
			Metadata: types.ArgocdApplicationMetadata{
				Name:      "web-app",
				Namespace: "argocd",
				Labels: map[string]string{
					"app.kubernetes.io/name": "web-app",
				},
			},
			Spec: types.ArgocdApplicationSpec{
				Project: "production",
				Source: types.ArgocdApplicationSource{
					RepoURL:        "https://github.com/company/web-app",
					Path:           "k8s",
					TargetRevision: "HEAD",
				},
				Destination: types.ArgocdApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "production",
				},
			},
			Status: types.ArgocdApplicationStatus{
				Health: types.ArgocdApplicationHealth{
					Status: "Healthy",
				},
				Sync: types.ArgocdApplicationSync{
					Status: "Synced",
				},
			},
		},
		{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "Application",
			Metadata: types.ArgocdApplicationMetadata{
				Name:      "api-service",
				Namespace: "argocd",
				Labels: map[string]string{
					"app.kubernetes.io/name": "api-service",
				},
			},
			Spec: types.ArgocdApplicationSpec{
				Project: "production",
				Source: types.ArgocdApplicationSource{
					RepoURL:        "https://github.com/company/api-service",
					Path:           "deploy",
					TargetRevision: "v1.2.3",
				},
				Destination: types.ArgocdApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "production",
				},
			},
			Status: types.ArgocdApplicationStatus{
				Health: types.ArgocdApplicationHealth{
					Status: "Healthy",
				},
				Sync: types.ArgocdApplicationSync{
					Status: "Synced",
				},
			},
		},
		{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "Application",
			Metadata: types.ArgocdApplicationMetadata{
				Name:      "test-app",
				Namespace: "argocd",
				Labels: map[string]string{
					"app.kubernetes.io/name": "test-app",
				},
			},
			Spec: types.ArgocdApplicationSpec{
				Project: "test-project",
				Source: types.ArgocdApplicationSource{
					RepoURL:        "https://github.com/company/test-app",
					Path:           "k8s",
					TargetRevision: "HEAD",
				},
				Destination: types.ArgocdApplicationDestination{
					Server:    "https://kubernetes.default.svc",
					Namespace: "test",
				},
			},
			Status: types.ArgocdApplicationStatus{
				Health: types.ArgocdApplicationHealth{
					Status: "Progressing",
				},
				Sync: types.ArgocdApplicationSync{
					Status: "OutOfSync",
				},
			},
		},
	}
}

// MockArgocdServer creates a test HTTP server that mocks ArgoCD API responses
type MockArgocdServer struct {
	server             *httptest.Server
	sessionResponses   map[string]*types.ArgocdSessionResponse
	projectResponses   map[string]*types.ArgocdProjectList
	appResponses       map[string]*types.ArgocdApplicationList
	appDetailResponses map[string]*types.ArgocdApplication
	errorResponses     map[string]int // endpoint -> status code
}

// NewMockArgocdServer creates a new mock ArgoCD server
func NewMockArgocdServer() *MockArgocdServer {
	mock := &MockArgocdServer{
		sessionResponses:   make(map[string]*types.ArgocdSessionResponse),
		projectResponses:   make(map[string]*types.ArgocdProjectList),
		appResponses:       make(map[string]*types.ArgocdApplicationList),
		appDetailResponses: make(map[string]*types.ArgocdApplication),
		errorResponses:     make(map[string]int),
	}

	// Set default responses
	mock.SetSessionResponse("POST", &types.ArgocdSessionResponse{
		Token: "mock-token-12345",
	})

	mock.SetProjectsResponse("GET", &types.ArgocdProjectList{
		APIVersion: "v1",
		Kind:       "List",
		Items:      CreateTestProjects(),
	})

	mock.SetApplicationsResponse("GET", &types.ArgocdApplicationList{
		APIVersion: "v1",
		Kind:       "List",
		Items:      CreateTestApplications(),
	})

	mock.server = httptest.NewServer(http.HandlerFunc(mock.handleRequest))
	return mock
}

// URL returns the base URL of the mock server
func (m *MockArgocdServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *MockArgocdServer) Close() {
	m.server.Close()
}

// SetSessionResponse sets the response for session endpoint
func (m *MockArgocdServer) SetSessionResponse(method string, response *types.ArgocdSessionResponse) {
	m.sessionResponses[method] = response
}

// SetProjectsResponse sets the response for projects endpoint
func (m *MockArgocdServer) SetProjectsResponse(method string, response *types.ArgocdProjectList) {
	m.projectResponses[method] = response
}

// SetApplicationsResponse sets the response for applications endpoint
func (m *MockArgocdServer) SetApplicationsResponse(method string, response *types.ArgocdApplicationList) {
	m.appResponses[method] = response
}

// SetApplicationDetailResponse sets the response for a specific application
func (m *MockArgocdServer) SetApplicationDetailResponse(appName string, response *types.ArgocdApplication) {
	m.appDetailResponses[appName] = response
}

// SetErrorResponse sets an error response for an endpoint
func (m *MockArgocdServer) SetErrorResponse(endpoint string, statusCode int) {
	m.errorResponses[endpoint] = statusCode
}

// handleRequest handles HTTP requests to the mock server
func (m *MockArgocdServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	endpoint := r.URL.Path
	method := r.Method

	// Check for error responses first
	if statusCode, exists := m.errorResponses[endpoint]; exists {
		w.WriteHeader(statusCode)
		w.Write([]byte(fmt.Sprintf("Mock error for %s", endpoint)))
		return
	}

	// Handle different endpoints
	switch {
	case endpoint == "/session" && method == "POST":
		if response, exists := m.sessionResponses[method]; exists {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}

	case endpoint == "/projects" && method == "GET":
		if response, exists := m.projectResponses[method]; exists {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}

	case endpoint == "/applications" && method == "GET":
		if response, exists := m.appResponses[method]; exists {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}

	case strings.HasPrefix(endpoint, "/applications/") && method == "GET":
		appName := strings.TrimPrefix(endpoint, "/applications/")
		if response, exists := m.appDetailResponses[appName]; exists {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(fmt.Sprintf("Application '%s' not found", appName)))
		}

	default:
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Mock endpoint not found"))
	}
}

// AssertJSONResponse verifies that an HTTP response contains expected JSON
func AssertJSONResponse(t *testing.T, response *http.Response, expectedStatus int, target interface{}) {
	t.Helper()

	if response.StatusCode != expectedStatus {
		t.Errorf("Expected status %d, got %d", expectedStatus, response.StatusCode)
		return
	}

	if target != nil {
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			t.Errorf("Failed to decode JSON response: %v", err)
		}
	}
}

// AssertStringContains verifies that a string contains expected substring
func AssertStringContains(t *testing.T, actual, expected, message string) {
	t.Helper()
	if !strings.Contains(actual, expected) {
		t.Errorf("%s: expected '%s' to contain '%s'", message, actual, expected)
	}
}

// AssertStringNotContains verifies that a string does not contain substring
func AssertStringNotContains(t *testing.T, actual, notExpected, message string) {
	t.Helper()
	if strings.Contains(actual, notExpected) {
		t.Errorf("%s: expected '%s' to not contain '%s'", message, actual, notExpected)
	}
}

// AssertSliceLength verifies that a slice has expected length
func AssertSliceLength(t *testing.T, slice interface{}, expectedLength int, message string) {
	t.Helper()
	var actualLength int

	switch s := slice.(type) {
	case []string:
		actualLength = len(s)
	case []types.ArgocdProject:
		actualLength = len(s)
	case []types.ArgocdApplication:
		actualLength = len(s)
	default:
		t.Errorf("AssertSliceLength: unsupported slice type")
		return
	}

	if actualLength != expectedLength {
		t.Errorf("%s: expected length %d, got %d", message, expectedLength, actualLength)
	}
}

// WaitForCondition waits for a condition to be true with timeout
func WaitForCondition(t *testing.T, condition func() bool, timeout time.Duration, message string) {
	t.Helper()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	timeoutChan := time.After(timeout)

	for {
		select {
		case <-ticker.C:
			if condition() {
				return
			}
		case <-timeoutChan:
			t.Errorf("Timeout waiting for condition: %s", message)
			return
		}
	}
}

// CreateTempFile creates a temporary file with content and returns cleanup function
func CreateTempFile(t *testing.T, filename, content string) func() {
	t.Helper()

	err := os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp file %s: %v", filename, err)
	}

	return func() {
		os.Remove(filename)
	}
}

// CreateTempDir creates a temporary directory and returns cleanup function
func CreateTempDir(t *testing.T, dirname string) func() {
	t.Helper()

	err := os.MkdirAll(dirname, 0755)
	if err != nil {
		t.Fatalf("Failed to create temp directory %s: %v", dirname, err)
	}

	return func() {
		os.RemoveAll(dirname)
	}
}

// RequestWithTimeout creates an HTTP request with timeout context
func RequestWithTimeout(method, url string, body []byte, timeout time.Duration) (*http.Request, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	var reqBody *bytes.Buffer
	if body != nil {
		reqBody = bytes.NewBuffer(body)
	}

	req, _ := http.NewRequestWithContext(ctx, method, url, reqBody)
	return req, cancel
}

// ConcurrentTest runs a function concurrently multiple times
func ConcurrentTest(t *testing.T, fn func(int), concurrency int, message string) {
	t.Helper()

	done := make(chan bool, concurrency)
	errors := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer func() {
				if r := recover(); r != nil {
					errors <- fmt.Errorf("goroutine %d panicked: %v", id, r)
				}
				done <- true
			}()
			fn(id)
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < concurrency; i++ {
		<-done
	}

	// Check for errors
	close(errors)
	for err := range errors {
		t.Errorf("%s: %v", message, err)
	}
}

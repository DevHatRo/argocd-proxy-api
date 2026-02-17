package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"argocd-proxy/config"
	"argocd-proxy/types"
)

// MockAuthService implements a mock auth service for testing
type MockAuthService struct {
	token   string
	err     error
	callLog []string
}

func (m *MockAuthService) GetValidToken(ctx context.Context) (string, error) {
	m.callLog = append(m.callLog, "GetValidToken")
	return m.token, m.err
}

func (m *MockAuthService) CreateAuthenticatedRequest(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	m.callLog = append(m.callLog, fmt.Sprintf("CreateAuthenticatedRequest-%s", method))

	if m.err != nil {
		return nil, m.err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", m.token))
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

func (m *MockAuthService) GetTokenStatus() map[string]interface{} {
	m.callLog = append(m.callLog, "GetTokenStatus")
	return map[string]interface{}{
		"hasToken": m.token != "",
		"isValid":  m.err == nil,
	}
}

func (m *MockAuthService) StartTokenRefreshRoutine(ctx context.Context) {
	m.callLog = append(m.callLog, "StartTokenRefreshRoutine")
	// Mock implementation - do nothing
}

// MockArgocdService implements ArgocdServiceInterface for testing
type MockArgocdService struct {
	projects     []types.ArgocdProject
	applications types.ArgocdApplicationList
	application  types.ArgocdApplication
	projectNames []string
	err          error
	healthErr    error
	ingressURLs  []string // New field for ingress URLs
}

func (m *MockArgocdService) GetProjects(ctx context.Context) ([]types.ArgocdProject, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.projects, nil
}

func (m *MockArgocdService) GetFilteredProjects(ctx context.Context) ([]types.ArgocdProject, error) {
	if m.err != nil {
		return nil, m.err
	}
	var filteredProjects []types.ArgocdProject
	for _, project := range m.projects {
		// Mock simple filtering logic for tests - in real implementation this uses ShouldFilterProject
		if !strings.HasPrefix(project.Metadata.Name, "test-") && !strings.HasSuffix(project.Metadata.Name, "-app") {
			filteredProjects = append(filteredProjects, project)
		}
	}
	return filteredProjects, nil
}

func (m *MockArgocdService) GetApplications(ctx context.Context) (types.ArgocdApplicationList, error) {
	if m.err != nil {
		return types.ArgocdApplicationList{}, m.err
	}
	return m.applications, nil
}

func (m *MockArgocdService) GetApplication(ctx context.Context, appName string) (types.ArgocdApplication, error) {
	if m.err != nil {
		return types.ArgocdApplication{}, m.err
	}
	for _, app := range m.applications.Items {
		if app.Metadata.Name == appName {
			return app, nil
		}
	}
	return types.ArgocdApplication{}, fmt.Errorf("application not found")
}

func (m *MockArgocdService) ProxyRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}

	resp := httptest.NewRecorder()
	resp.Code = http.StatusOK // Default to OK for mock
	resp.Header().Set("Content-Type", "application/json")

	// Simulate different response codes based on path
	switch path {
	case "/projects":
		if method == "GET" {
			projectList := types.ArgocdProjectList{
				APIVersion: "v1",
				Kind:       "List",
				Items:      m.projects,
			}
			json.NewEncoder(resp).Encode(projectList)
		} else {
			resp.Code = http.StatusMethodNotAllowed
			resp.Write([]byte("Method Not Allowed"))
		}
	case "/applications":
		if method == "GET" {
			appList := types.ArgocdApplicationList{
				APIVersion: "v1",
				Kind:       "List",
				Items:      m.applications.Items,
			}
			json.NewEncoder(resp).Encode(appList)
		} else {
			resp.Code = http.StatusMethodNotAllowed
			resp.Write([]byte("Method Not Allowed"))
		}
	case "/applications/my-app": // Specific application retrieval
		if method == "GET" {
			json.NewEncoder(resp).Encode(m.application)
		} else {
			resp.Code = http.StatusMethodNotAllowed
			resp.Write([]byte("Method Not Allowed"))
		}
	default:
		resp.Code = http.StatusNotFound
		resp.Write([]byte("Not Found"))
	}

	return resp.Result(), nil
}

func (m *MockArgocdService) GetProjectNames(ctx context.Context) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.projectNames, nil
}

func (m *MockArgocdService) HealthCheck(ctx context.Context) error {
	if m.healthErr != nil {
		return m.healthErr
	}
	return nil
}

func (m *MockArgocdService) ExtractIngressURLs(ctx context.Context, appName string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.ingressURLs, nil
}

func (m *MockArgocdService) GetApplicationsByGroup(ctx context.Context, groupName string, cfg interface{}) (types.ArgocdApplicationList, error) {
	if m.err != nil {
		return types.ArgocdApplicationList{}, m.err
	}
	// Return mock applications for testing
	return m.applications, nil
}

func (m *MockArgocdService) GetApplicationsByProject(ctx context.Context, projectName string) (types.ArgocdApplicationList, error) {
	if m.err != nil {
		return types.ArgocdApplicationList{}, m.err
	}
	// Filter applications by project for testing
	var filteredApps []types.ArgocdApplication
	for _, app := range m.applications.Items {
		if app.Spec.Project == projectName {
			filteredApps = append(filteredApps, app)
		}
	}
	return types.ArgocdApplicationList{
		APIVersion: m.applications.APIVersion,
		Kind:       m.applications.Kind,
		Items:      filteredApps,
		Metadata:   m.applications.Metadata,
	}, nil
}

func TestNewArgocdService(t *testing.T) {
	cfg := &config.Config{
		ArgocdAPIURL:   "https://argocd.example.com/api/v1",
		ArgocdUsername: "testuser",
		ArgocdPassword: "testpass",
	}
	authSvc := &MockAuthService{token: "test-token"}

	service := NewArgocdService(cfg, authSvc)

	if service.config != cfg {
		t.Errorf("NewArgocdService() config not set correctly")
	}
	if service.authService == nil {
		t.Errorf("NewArgocdService() authService not set correctly")
	}
	if service.httpClient == nil {
		t.Errorf("NewArgocdService() httpClient not initialized")
	}
	if service.httpClient.Timeout != 10*time.Second {
		t.Errorf("NewArgocdService() httpClient timeout = %v, want %v", service.httpClient.Timeout, 10*time.Second)
	}
}

func TestGetProjects(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectError    bool
		expectedCount  int
	}{
		{
			name: "successful projects retrieval",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
				}
				if r.URL.Path != "/projects" {
					t.Errorf("Expected /projects path, got %s", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("Expected Authorization header with Bearer test-token")
				}

				projectList := types.ArgocdProjectList{
					APIVersion: "v1",
					Kind:       "List",
					Items: []types.ArgocdProject{
						{
							APIVersion: "argoproj.io/v1alpha1",
							Kind:       "AppProject",
							Metadata:   types.ArgocdProjectMetadata{Name: "project1"},
						},
						{
							APIVersion: "argoproj.io/v1alpha1",
							Kind:       "AppProject",
							Metadata:   types.ArgocdProjectMetadata{Name: "project2"},
						},
					},
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(projectList)
			},
			expectError:   false,
			expectedCount: 2,
		},
		{
			name: "ArgoCD API error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
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

			cfg := &config.Config{ArgocdAPIURL: server.URL}
			authSvc := &MockAuthService{token: "test-token"}
			service := NewArgocdService(cfg, authSvc)

			ctx := context.Background()
			projects, err := service.GetProjects(ctx)

			if tt.expectError {
				if err == nil {
					t.Errorf("GetProjects() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("GetProjects() unexpected error: %v", err)
				return
			}

			if len(projects) != tt.expectedCount {
				t.Errorf("GetProjects() count = %v, want %v", len(projects), tt.expectedCount)
			}
		})
	}
}

func TestGetFilteredProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectList := types.ArgocdProjectList{
			Items: []types.ArgocdProject{
				{Metadata: types.ArgocdProjectMetadata{Name: "production"}},
				{Metadata: types.ArgocdProjectMetadata{Name: "test-project"}},
				{Metadata: types.ArgocdProjectMetadata{Name: "staging-app"}},
				{Metadata: types.ArgocdProjectMetadata{Name: "development"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(projectList)
	}))
	defer server.Close()

	cfg := &config.Config{
		ArgocdAPIURL:    server.URL,
		IgnoredProjects: []string{"test-*", "*-app"},
	}
	authSvc := &MockAuthService{token: "test-token"}
	service := NewArgocdService(cfg, authSvc)

	ctx := context.Background()
	projects, err := service.GetFilteredProjects(ctx)

	if err != nil {
		t.Errorf("GetFilteredProjects() error = %v", err)
		return
	}

	expectedProjects := []string{"production", "development"}
	if len(projects) != len(expectedProjects) {
		t.Errorf("GetFilteredProjects() count = %v, want %v", len(projects), len(expectedProjects))
		return
	}

	for i, project := range projects {
		if project.Metadata.Name != expectedProjects[i] {
			t.Errorf("GetFilteredProjects() project[%d] = %v, want %v", i, project.Metadata.Name, expectedProjects[i])
		}
	}
}

func TestGetApplications(t *testing.T) {
	tests := []struct {
		name             string
		ignoredProjects  []string
		serverResponse   func(w http.ResponseWriter, r *http.Request)
		expectError      bool
		expectedAppCount int
		expectedAppNames []string
	}{
		{
			name:            "successful applications retrieval with filtering",
			ignoredProjects: []string{"test-*"},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/applications" {
					t.Errorf("Expected /applications path, got %s", r.URL.Path)
				}

				appList := types.ArgocdApplicationList{
					APIVersion: "v1",
					Kind:       "List",
					Items: []types.ArgocdApplication{
						{
							Metadata: types.ArgocdApplicationMetadata{Name: "prod-app"},
							Spec:     types.ArgocdApplicationSpec{Project: "production"},
						},
						{
							Metadata: types.ArgocdApplicationMetadata{Name: "test-app"},
							Spec:     types.ArgocdApplicationSpec{Project: "test-project"},
						},
						{
							Metadata: types.ArgocdApplicationMetadata{Name: "staging-app"},
							Spec:     types.ArgocdApplicationSpec{Project: "staging"},
						},
					},
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(appList)
			},
			expectError:      false,
			expectedAppCount: 2, // test-app should be filtered out
			expectedAppNames: []string{"prod-app", "staging-app"},
		},
		{
			name: "ArgoCD API error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad Request"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := &config.Config{
				ArgocdAPIURL:    server.URL,
				IgnoredProjects: tt.ignoredProjects,
			}
			authSvc := &MockAuthService{token: "test-token"}
			service := NewArgocdService(cfg, authSvc)

			ctx := context.Background()
			appList, err := service.GetApplications(ctx)

			if tt.expectError {
				if err == nil {
					t.Errorf("GetApplications() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("GetApplications() unexpected error: %v", err)
				return
			}

			if len(appList.Items) != tt.expectedAppCount {
				t.Errorf("GetApplications() count = %v, want %v", len(appList.Items), tt.expectedAppCount)
				return
			}

			for i, expectedName := range tt.expectedAppNames {
				if i < len(appList.Items) && appList.Items[i].Metadata.Name != expectedName {
					t.Errorf("GetApplications() app[%d] name = %v, want %v", i, appList.Items[i].Metadata.Name, expectedName)
				}
			}
		})
	}
}

func TestGetApplication(t *testing.T) {
	tests := []struct {
		name            string
		appName         string
		ignoredProjects []string
		serverResponse  func(w http.ResponseWriter, r *http.Request)
		expectError     bool
		expectedApp     string
		errorContains   string
	}{
		{
			name:    "successful application retrieval",
			appName: "my-app",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/applications/my-app" {
					t.Errorf("Expected /applications/my-app path, got %s", r.URL.Path)
				}

				app := types.ArgocdApplication{
					Metadata: types.ArgocdApplicationMetadata{Name: "my-app"},
					Spec:     types.ArgocdApplicationSpec{Project: "production"},
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(app)
			},
			expectedApp: "my-app",
		},
		{
			name:            "application in filtered project",
			appName:         "test-app",
			ignoredProjects: []string{"test-*"},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				app := types.ArgocdApplication{
					Metadata: types.ArgocdApplicationMetadata{Name: "test-app"},
					Spec:     types.ArgocdApplicationSpec{Project: "test-project"},
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(app)
			},
			expectError:   true,
			errorContains: "filtered project",
		},
		{
			name:    "application not found",
			appName: "nonexistent",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("Application not found"))
			},
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:    "ArgoCD API error",
			appName: "error-app",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := &config.Config{
				ArgocdAPIURL:    server.URL,
				IgnoredProjects: tt.ignoredProjects,
			}
			authSvc := &MockAuthService{token: "test-token"}
			service := NewArgocdService(cfg, authSvc)

			ctx := context.Background()
			app, err := service.GetApplication(ctx, tt.appName)

			if tt.expectError {
				if err == nil {
					t.Errorf("GetApplication() expected error but got none")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("GetApplication() error = %v, should contain %s", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("GetApplication() unexpected error: %v", err)
				return
			}

			if app.Metadata.Name != tt.expectedApp {
				t.Errorf("GetApplication() app name = %v, want %v", app.Metadata.Name, tt.expectedApp)
			}
		})
	}
}

func TestProxyRequest(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		body           []byte
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectError    bool
	}{
		{
			name:   "successful GET request",
			method: "GET",
			path:   "/projects",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
				}
				if r.URL.Path != "/projects" {
					t.Errorf("Expected /projects path, got %s", r.URL.Path)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"result":"success"}`))
			},
		},
		{
			name:   "successful POST request",
			method: "POST",
			path:   "/applications",
			body:   []byte(`{"name":"test"}`),
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
				}
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"created":"ok"}`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := &config.Config{ArgocdAPIURL: server.URL}
			authSvc := &MockAuthService{token: "test-token"}
			service := NewArgocdService(cfg, authSvc)

			ctx := context.Background()
			resp, err := service.ProxyRequest(ctx, tt.method, tt.path, tt.body)

			if tt.expectError {
				if err == nil {
					t.Errorf("ProxyRequest() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("ProxyRequest() unexpected error: %v", err)
				return
			}

			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				t.Errorf("ProxyRequest() status = %v, want 2xx", resp.StatusCode)
			}
		})
	}
}

func TestGetProjectNames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectList := types.ArgocdProjectList{
			Items: []types.ArgocdProject{
				{Metadata: types.ArgocdProjectMetadata{Name: "project1"}},
				{Metadata: types.ArgocdProjectMetadata{Name: "project2"}},
				{Metadata: types.ArgocdProjectMetadata{Name: "project3"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(projectList)
	}))
	defer server.Close()

	cfg := &config.Config{ArgocdAPIURL: server.URL}
	authSvc := &MockAuthService{token: "test-token"}
	service := NewArgocdService(cfg, authSvc)

	ctx := context.Background()
	names, err := service.GetProjectNames(ctx)

	if err != nil {
		t.Errorf("GetProjectNames() error = %v", err)
		return
	}

	expectedNames := []string{"project1", "project2", "project3"}
	if len(names) != len(expectedNames) {
		t.Errorf("GetProjectNames() count = %v, want %v", len(names), len(expectedNames))
		return
	}

	for i, expectedName := range expectedNames {
		if names[i] != expectedName {
			t.Errorf("GetProjectNames() name[%d] = %v, want %v", i, names[i], expectedName)
		}
	}
}

func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name           string
		authError      error
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectError    bool
		errorContains  string
	}{
		{
			name: "healthy service",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"items":[]}`))
			},
			expectError: false,
		},
		{
			name:          "auth service error",
			authError:     fmt.Errorf("token refresh failed"),
			expectError:   true,
			errorContains: "token validation failed",
		},
		{
			name: "ArgoCD API unreachable",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("Service Unavailable"))
			},
			expectError:   true,
			errorContains: "health check failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.serverResponse != nil {
				server = httptest.NewServer(http.HandlerFunc(tt.serverResponse))
				defer server.Close()
			}

			cfg := &config.Config{}
			if server != nil {
				cfg.ArgocdAPIURL = server.URL
			}

			authSvc := &MockAuthService{
				token: "test-token",
				err:   tt.authError,
			}
			service := NewArgocdService(cfg, authSvc)

			ctx := context.Background()
			err := service.HealthCheck(ctx)

			if tt.expectError {
				if err == nil {
					t.Errorf("HealthCheck() expected error but got none")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("HealthCheck() error = %v, should contain %s", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("HealthCheck() unexpected error: %v", err)
			}
		})
	}
}

func TestContextTimeout(t *testing.T) {
	// Server that delays response to test timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // Delay longer than context timeout
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	cfg := &config.Config{ArgocdAPIURL: server.URL}
	authSvc := &MockAuthService{token: "test-token"}
	service := NewArgocdService(cfg, authSvc)

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := service.GetProjects(ctx)
	if err == nil {
		t.Errorf("GetProjects() expected timeout error but got none")
	}

	// Check that the error is related to context cancellation
	if !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "timeout") {
		t.Errorf("GetProjects() error should be context-related, got: %v", err)
	}
}

func TestAuthServiceIntegration(t *testing.T) {
	// Test that the service properly uses the auth service
	authCallCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer integration-token" {
			t.Errorf("Expected Authorization header with Bearer integration-token, got %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	cfg := &config.Config{ArgocdAPIURL: server.URL}
	authSvc := &MockAuthService{token: "integration-token"}
	service := NewArgocdService(cfg, authSvc)

	ctx := context.Background()

	// Make multiple calls to verify auth service is used
	service.GetProjects(ctx)
	authCallCount++

	service.GetApplications(ctx)
	authCallCount++

	// Verify auth service was called
	expectedCalls := authCallCount // Each method calls CreateAuthenticatedRequest once
	if len(authSvc.callLog) < expectedCalls {
		t.Errorf("Expected at least %d auth service calls, got %d", expectedCalls, len(authSvc.callLog))
	}
}

// Benchmark tests for performance-critical functions
func BenchmarkGetProjects(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectList := types.ArgocdProjectList{
			Items: make([]types.ArgocdProject, 100), // 100 projects
		}
		for i := 0; i < 100; i++ {
			projectList.Items[i] = types.ArgocdProject{
				Metadata: types.ArgocdProjectMetadata{Name: fmt.Sprintf("project-%d", i)},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(projectList)
	}))
	defer server.Close()

	cfg := &config.Config{ArgocdAPIURL: server.URL}
	authSvc := &MockAuthService{token: "bench-token"}
	service := NewArgocdService(cfg, authSvc)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.GetProjects(ctx)
	}
}

func BenchmarkGetApplicationsWithFiltering(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appList := types.ArgocdApplicationList{
			Items: make([]types.ArgocdApplication, 200), // 200 applications
		}
		for i := 0; i < 200; i++ {
			project := "production"
			if i%5 == 0 {
				project = "test-project" // Will be filtered out
			}
			appList.Items[i] = types.ArgocdApplication{
				Metadata: types.ArgocdApplicationMetadata{Name: fmt.Sprintf("app-%d", i)},
				Spec:     types.ArgocdApplicationSpec{Project: project},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(appList)
	}))
	defer server.Close()

	cfg := &config.Config{
		ArgocdAPIURL:    server.URL,
		IgnoredProjects: []string{"test-*"},
	}
	authSvc := &MockAuthService{token: "bench-token"}
	service := NewArgocdService(cfg, authSvc)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.GetApplications(ctx)
	}
}

func TestGetProjectsCaching(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		projectList := types.ArgocdProjectList{
			Items: []types.ArgocdProject{
				{Metadata: types.ArgocdProjectMetadata{Name: "project-1"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(projectList)
	}))
	defer server.Close()

	cfg := &config.Config{
		ArgocdAPIURL: server.URL,
		CacheTTL:     30 * time.Second,
	}
	authSvc := &MockAuthService{token: "test-token"}
	service := NewArgocdService(cfg, authSvc)
	ctx := context.Background()

	// First call should hit the server
	projects, err := service.GetProjects(ctx)
	if err != nil {
		t.Fatalf("first GetProjects() error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("first GetProjects() count = %d, want 1", len(projects))
	}
	if callCount != 1 {
		t.Fatalf("expected 1 server call, got %d", callCount)
	}

	// Second call should be served from cache
	projects, err = service.GetProjects(ctx)
	if err != nil {
		t.Fatalf("second GetProjects() error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("second GetProjects() count = %d, want 1", len(projects))
	}
	if callCount != 1 {
		t.Errorf("expected server to still have 1 call (cache hit), got %d", callCount)
	}
}

func TestGetApplicationsCaching(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		appList := types.ArgocdApplicationList{
			APIVersion: "v1",
			Kind:       "List",
			Items: []types.ArgocdApplication{
				{
					Metadata: types.ArgocdApplicationMetadata{Name: "app-1"},
					Spec:     types.ArgocdApplicationSpec{Project: "default"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(appList)
	}))
	defer server.Close()

	cfg := &config.Config{
		ArgocdAPIURL: server.URL,
		CacheTTL:     30 * time.Second,
	}
	authSvc := &MockAuthService{token: "test-token"}
	service := NewArgocdService(cfg, authSvc)
	ctx := context.Background()

	// First call hits server
	apps, err := service.GetApplications(ctx)
	if err != nil {
		t.Fatalf("first GetApplications() error: %v", err)
	}
	if len(apps.Items) != 1 {
		t.Fatalf("first GetApplications() count = %d, want 1", len(apps.Items))
	}
	if callCount != 1 {
		t.Fatalf("expected 1 server call, got %d", callCount)
	}

	// Second call served from cache
	apps, err = service.GetApplications(ctx)
	if err != nil {
		t.Fatalf("second GetApplications() error: %v", err)
	}
	if len(apps.Items) != 1 {
		t.Fatalf("second GetApplications() count = %d, want 1", len(apps.Items))
	}
	if callCount != 1 {
		t.Errorf("expected server to still have 1 call (cache hit), got %d", callCount)
	}
}

func TestCacheDisabledWithZeroTTL(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		projectList := types.ArgocdProjectList{
			Items: []types.ArgocdProject{
				{Metadata: types.ArgocdProjectMetadata{Name: "project-1"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(projectList)
	}))
	defer server.Close()

	cfg := &config.Config{
		ArgocdAPIURL: server.URL,
		CacheTTL:     0,
	}
	authSvc := &MockAuthService{token: "test-token"}
	service := NewArgocdService(cfg, authSvc)
	ctx := context.Background()

	service.GetProjects(ctx)
	service.GetProjects(ctx)

	if callCount != 2 {
		t.Errorf("with TTL=0, expected 2 server calls, got %d", callCount)
	}
}

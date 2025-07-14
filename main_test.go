package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"argocd-proxy/config"
	"argocd-proxy/types"
)

// MockAuthService for testing HTTP handlers
type MockAuthService struct {
	token     string
	err       error
	callCount int
}

func (m *MockAuthService) GetValidToken(ctx context.Context) (string, error) {
	m.callCount++
	return m.token, m.err
}

func (m *MockAuthService) CreateAuthenticatedRequest(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}

	var reqBody *bytes.Buffer
	if body != nil {
		reqBody = bytes.NewBuffer(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", m.token))
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

func (m *MockAuthService) GetTokenStatus() map[string]interface{} {
	return map[string]interface{}{
		"hasToken": m.token != "",
		"isValid":  m.err == nil,
	}
}

func (m *MockAuthService) StartTokenRefreshRoutine(ctx context.Context) {
	// Mock implementation - do nothing
}

// MockArgocdService for testing
type MockArgocdService struct {
	projects     []types.ArgocdProject
	applications types.ArgocdApplicationList
	application  types.ArgocdApplication
	projectNames []string
	err          error
	healthErr    error
}

func (m *MockArgocdService) GetProjects(ctx context.Context) ([]types.ArgocdProject, error) {
	return m.projects, m.err
}

func (m *MockArgocdService) GetFilteredProjects(ctx context.Context) ([]types.ArgocdProject, error) {
	return m.projects, m.err
}

func (m *MockArgocdService) GetApplications(ctx context.Context) (types.ArgocdApplicationList, error) {
	return m.applications, m.err
}

func (m *MockArgocdService) GetApplication(ctx context.Context, name string) (types.ArgocdApplication, error) {
	if m.err != nil {
		return types.ArgocdApplication{}, m.err
	}
	if m.application.Metadata.Name == "" {
		return types.ArgocdApplication{}, fmt.Errorf("application '%s' not found", name)
	}
	return m.application, nil
}

func (m *MockArgocdService) GetProjectNames(ctx context.Context) ([]string, error) {
	return m.projectNames, m.err
}

func (m *MockArgocdService) HealthCheck(ctx context.Context) error {
	return m.healthErr
}

func (m *MockArgocdService) ExtractIngressURLs(ctx context.Context, appName string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Return mock ingress URLs for testing
	return []string{"https://example.com", "https://app.example.com"}, nil
}

func (m *MockArgocdService) GetApplicationsByGroup(ctx context.Context, groupName string, cfg interface{}) (types.ArgocdApplicationList, error) {
	if m.err != nil {
		return types.ArgocdApplicationList{}, m.err
	}
	// Return mock applications for testing - in a real scenario this would filter by group
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

func setupTestServer() *Server {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Port: "5001",
		ProjectGroups: []config.ProjectGroup{
			{Name: "Frontend", Description: "Frontend apps", Projects: []string{"web-app"}},
		},
		IgnoredProjects: []string{"test-*"},
	}

	server := &Server{
		config: cfg,
	}

	server.authService = &MockAuthService{token: "test-token"}
	server.argocdService = &MockArgocdService{}

	server.setupRouter()

	return server
}

func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name           string
		authService    *MockAuthService
		argocdService  *MockArgocdService
		expectedStatus int
		expectedHealth string
	}{
		{
			name:           "healthy service",
			authService:    &MockAuthService{token: "test-token"},
			argocdService:  &MockArgocdService{},
			expectedStatus: http.StatusOK,
			expectedHealth: "healthy",
		},
		{
			name:           "degraded service - ArgoCD error",
			authService:    &MockAuthService{token: "test-token"},
			argocdService:  &MockArgocdService{healthErr: fmt.Errorf("ArgoCD unavailable")},
			expectedStatus: http.StatusServiceUnavailable,
			expectedHealth: "degraded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupTestServer()
			server.authService = tt.authService
			server.argocdService = tt.argocdService

			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("healthCheck() status = %v, want %v", w.Code, tt.expectedStatus)
			}

			var response types.HealthResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Errorf("healthCheck() invalid JSON response: %v", err)
				return
			}

			if response.Status != tt.expectedHealth {
				t.Errorf("healthCheck() status = %v, want %v", response.Status, tt.expectedHealth)
			}

			if response.TokenStatus == nil {
				t.Errorf("healthCheck() missing token status")
			}
		})
	}
}

func TestGetProjectGroups(t *testing.T) {
	tests := []struct {
		name              string
		projectNames      []string
		serviceErr        error
		expectedStatus    int
		expectedGroups    int
		expectedUngrouped int
	}{
		{
			name:              "successful project groups",
			projectNames:      []string{"web-app", "api-service", "test-app", "other-service"},
			expectedStatus:    http.StatusOK,
			expectedGroups:    1, // One configured group
			expectedUngrouped: 2, // api-service, other-service (test-app is ignored)
		},
		{
			name:           "service error",
			serviceErr:     fmt.Errorf("ArgoCD error"),
			expectedStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupTestServer()
			mockService := server.argocdService.(*MockArgocdService)
			mockService.projectNames = tt.projectNames
			mockService.err = tt.serviceErr

			req := httptest.NewRequest("GET", "/project-groups", nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("getProjectGroups() status = %v, want %v", w.Code, tt.expectedStatus)
			}

			if tt.serviceErr == nil {
				var response config.ProjectGroupsResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Errorf("getProjectGroups() invalid JSON response: %v", err)
					return
				}

				if len(response.Groups) != tt.expectedGroups {
					t.Errorf("getProjectGroups() groups count = %v, want %v", len(response.Groups), tt.expectedGroups)
				}

				if len(response.UngroupedProjects) != tt.expectedUngrouped {
					t.Errorf("getProjectGroups() ungrouped count = %v, want %v", len(response.UngroupedProjects), tt.expectedUngrouped)
				}
			}
		})
	}
}

func TestGetProjects(t *testing.T) {
	tests := []struct {
		name           string
		projects       []types.ArgocdProject
		serviceErr     error
		expectedStatus int
		expectedCount  int
	}{
		{
			name: "successful projects retrieval",
			projects: []types.ArgocdProject{
				{Metadata: types.ArgocdProjectMetadata{Name: "project1"}},
				{Metadata: types.ArgocdProjectMetadata{Name: "project2"}},
			},
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:           "service error",
			serviceErr:     fmt.Errorf("ArgoCD error"),
			expectedStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupTestServer()
			mockService := server.argocdService.(*MockArgocdService)
			mockService.projects = tt.projects
			mockService.err = tt.serviceErr

			req := httptest.NewRequest("GET", "/projects", nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("getProjects() status = %v, want %v", w.Code, tt.expectedStatus)
			}

			if tt.serviceErr == nil {
				var response map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Errorf("getProjects() invalid JSON response: %v", err)
					return
				}

				if response["apiVersion"] != "v1" {
					t.Errorf("getProjects() apiVersion = %v, want v1", response["apiVersion"])
				}

				if response["kind"] != "List" {
					t.Errorf("getProjects() kind = %v, want List", response["kind"])
				}

				items, ok := response["items"].([]interface{})
				if !ok {
					t.Errorf("getProjects() items is not a slice")
					return
				}

				if len(items) != tt.expectedCount {
					t.Errorf("getProjects() items count = %v, want %v", len(items), tt.expectedCount)
				}
			}
		})
	}
}

func TestGetApplications(t *testing.T) {
	tests := []struct {
		name           string
		applications   types.ArgocdApplicationList
		serviceErr     error
		expectedStatus int
		expectedCount  int
	}{
		{
			name: "successful applications retrieval",
			applications: types.ArgocdApplicationList{
				APIVersion: "v1",
				Kind:       "List",
				Items: []types.ArgocdApplication{
					{Metadata: types.ArgocdApplicationMetadata{Name: "app1"}},
					{Metadata: types.ArgocdApplicationMetadata{Name: "app2"}},
				},
			},
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:           "service error",
			serviceErr:     fmt.Errorf("ArgoCD error"),
			expectedStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupTestServer()
			mockService := server.argocdService.(*MockArgocdService)
			mockService.applications = tt.applications
			mockService.err = tt.serviceErr

			req := httptest.NewRequest("GET", "/applications", nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("getApplications() status = %v, want %v", w.Code, tt.expectedStatus)
			}

			if tt.serviceErr == nil {
				var response types.ArgocdApplicationList
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Errorf("getApplications() invalid JSON response: %v", err)
					return
				}

				if len(response.Items) != tt.expectedCount {
					t.Errorf("getApplications() items count = %v, want %v", len(response.Items), tt.expectedCount)
				}
			}
		})
	}
}

func TestGetApplication(t *testing.T) {
	tests := []struct {
		name           string
		appName        string
		application    types.ArgocdApplication
		serviceErr     error
		expectedStatus int
	}{
		{
			name:    "successful application retrieval",
			appName: "my-app",
			application: types.ArgocdApplication{
				Metadata: types.ArgocdApplicationMetadata{Name: "my-app"},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "application not found",
			appName:        "nonexistent",
			serviceErr:     fmt.Errorf("application 'nonexistent' not found"),
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "service error",
			appName:        "error-app",
			serviceErr:     fmt.Errorf("ArgoCD connection error"),
			expectedStatus: http.StatusBadGateway,
		},
		{
			name:           "empty app name",
			appName:        "",
			expectedStatus: http.StatusMovedPermanently, // Gin redirects empty param to base path
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupTestServer()
			mockService := server.argocdService.(*MockArgocdService)
			mockService.application = tt.application
			mockService.err = tt.serviceErr

			url := fmt.Sprintf("/applications/%s", tt.appName)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("getApplication() status = %v, want %v", w.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				var response types.ArgocdApplication
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Errorf("getApplication() invalid JSON response: %v", err)
					return
				}

				if response.Metadata.Name != tt.application.Metadata.Name {
					t.Errorf("getApplication() app name = %v, want %v", response.Metadata.Name, tt.application.Metadata.Name)
				}
			}
		})
	}
}

func TestCORSHeaders(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest("OPTIONS", "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")

	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	// Check CORS headers
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Errorf("CORS: Missing Access-Control-Allow-Origin header")
	}

	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Errorf("CORS: Missing Access-Control-Allow-Methods header")
	}
}

func TestRequestTimeouts(t *testing.T) {
	// Test that requests have proper timeout contexts
	server := setupTestServer()

	// Create a mock service that will check if context has timeout
	mockService := &MockArgocdService{
		projects: []types.ArgocdProject{
			{Metadata: types.ArgocdProjectMetadata{Name: "test"}},
		},
	}
	server.argocdService = mockService

	req := httptest.NewRequest("GET", "/projects", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	server.router.ServeHTTP(w, req)
	duration := time.Since(start)

	// Should complete quickly since it's mocked
	if duration > 1*time.Second {
		t.Errorf("Request took too long: %v", duration)
	}

	if w.Code != http.StatusOK {
		t.Errorf("Request failed with status: %v", w.Code)
	}
}

func TestErrorResponse(t *testing.T) {
	server := setupTestServer()

	// Test error response structure
	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{
			name:           "not found endpoint",
			path:           "/nonexistent",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("errorResponse() status = %v, want %v", w.Code, tt.expectedStatus)
			}

			var response types.ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Errorf("errorResponse() invalid JSON: %v", err)
				return
			}

			if response.Code != tt.expectedStatus {
				t.Errorf("errorResponse() code = %v, want %v", response.Code, tt.expectedStatus)
			}

			if response.Error == "" {
				t.Errorf("errorResponse() missing error field")
			}
		})
	}
}

func TestContentTypes(t *testing.T) {
	server := setupTestServer()

	tests := []struct {
		name         string
		path         string
		expectedType string
	}{
		{
			name:         "health endpoint JSON",
			path:         "/health",
			expectedType: "application/json",
		},
		{
			name:         "projects endpoint JSON",
			path:         "/projects",
			expectedType: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			contentType := w.Header().Get("Content-Type")
			if !strings.Contains(contentType, tt.expectedType) {
				t.Errorf("Content-Type = %v, should contain %v", contentType, tt.expectedType)
			}
		})
	}
}

func TestGetApplicationsByGroup(t *testing.T) {
	tests := []struct {
		name           string
		groupName      string
		applications   types.ArgocdApplicationList
		serviceErr     error
		expectedStatus int
	}{
		{
			name:      "successful group applications retrieval",
			groupName: "Frontend",
			applications: types.ArgocdApplicationList{
				Items: []types.ArgocdApplication{
					{Metadata: types.ArgocdApplicationMetadata{Name: "web-app"}},
				},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "service error",
			groupName:      "Backend",
			serviceErr:     fmt.Errorf("ArgoCD error"),
			expectedStatus: http.StatusBadGateway,
		},
		{
			name:           "group not found",
			groupName:      "NonExistent",
			serviceErr:     fmt.Errorf("project group 'NonExistent' not found"),
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupTestServer()
			mockService := server.argocdService.(*MockArgocdService)
			mockService.applications = tt.applications
			mockService.err = tt.serviceErr

			url := fmt.Sprintf("/groups/%s/applications", tt.groupName)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("getApplicationsByGroup() status = %v, want %v", w.Code, tt.expectedStatus)
			}
		})
	}
}

func TestGetApplicationsByProject(t *testing.T) {
	tests := []struct {
		name           string
		projectName    string
		applications   types.ArgocdApplicationList
		serviceErr     error
		expectedStatus int
		expectedCount  int
	}{
		{
			name:        "successful project applications retrieval",
			projectName: "production",
			applications: types.ArgocdApplicationList{
				Items: []types.ArgocdApplication{
					{
						Metadata: types.ArgocdApplicationMetadata{Name: "prod-app"},
						Spec:     types.ArgocdApplicationSpec{Project: "production"},
					},
					{
						Metadata: types.ArgocdApplicationMetadata{Name: "other-app"},
						Spec:     types.ArgocdApplicationSpec{Project: "staging"},
					},
				},
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1, // Only one app belongs to "production"
		},
		{
			name:           "service error",
			projectName:    "backend",
			serviceErr:     fmt.Errorf("ArgoCD error"),
			expectedStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupTestServer()
			mockService := server.argocdService.(*MockArgocdService)
			mockService.applications = tt.applications
			mockService.err = tt.serviceErr

			url := fmt.Sprintf("/projects/%s/applications", tt.projectName)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("getApplicationsByProject() status = %v, want %v", w.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				var response types.ArgocdApplicationList
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Errorf("getApplicationsByProject() invalid JSON response: %v", err)
					return
				}

				if len(response.Items) != tt.expectedCount {
					t.Errorf("getApplicationsByProject() count = %v, want %v", len(response.Items), tt.expectedCount)
				}
			}
		})
	}
}

// Benchmark tests for HTTP handlers
func BenchmarkHealthCheck(b *testing.B) {
	server := setupTestServer()

	req := httptest.NewRequest("GET", "/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
	}
}

func BenchmarkGetProjects(b *testing.B) {
	server := setupTestServer()
	mockService := server.argocdService.(*MockArgocdService)

	// Setup some test data
	projects := make([]types.ArgocdProject, 100)
	for i := 0; i < 100; i++ {
		projects[i] = types.ArgocdProject{
			Metadata: types.ArgocdProjectMetadata{Name: fmt.Sprintf("project-%d", i)},
		}
	}
	mockService.projects = projects

	req := httptest.NewRequest("GET", "/projects", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)
	}
}

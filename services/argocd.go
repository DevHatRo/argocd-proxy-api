package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"argocd-proxy/config"
	"argocd-proxy/types"
)

// ArgocdService provides access to ArgoCD API endpoints
type ArgocdService struct {
	config      *config.Config
	authService types.AuthServiceInterface
	httpClient  *http.Client
}

// NewArgocdService creates a new ArgoCD service instance
func NewArgocdService(cfg *config.Config, authSvc types.AuthServiceInterface) *ArgocdService {
	return &ArgocdService{
		config:      cfg,
		authService: authSvc,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetProjects retrieves all projects from ArgoCD
func (s *ArgocdService) GetProjects(ctx context.Context) ([]types.ArgocdProject, error) {
	url := fmt.Sprintf("%s/projects", s.config.ArgocdAPIURL)

	req, err := s.authService.CreateAuthenticatedRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request to ArgoCD: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ArgoCD API returned status %d: %s", resp.StatusCode, string(body))
	}

	var projectList types.ArgocdProjectList
	if err := json.NewDecoder(resp.Body).Decode(&projectList); err != nil {
		return nil, fmt.Errorf("failed to decode projects response: %w", err)
	}

	return projectList.Items, nil
}

// GetFilteredProjects retrieves projects from ArgoCD with filtering applied
func (s *ArgocdService) GetFilteredProjects(ctx context.Context) ([]types.ArgocdProject, error) {
	projects, err := s.GetProjects(ctx)
	if err != nil {
		return nil, err
	}

	var filteredProjects []types.ArgocdProject
	for _, project := range projects {
		if !s.config.ShouldFilterProject(project.Metadata.Name) {
			filteredProjects = append(filteredProjects, project)
		}
	}

	return filteredProjects, nil
}

// GetApplications retrieves all applications from ArgoCD with filtering applied
func (s *ArgocdService) GetApplications(ctx context.Context) (types.ArgocdApplicationList, error) {
	url := fmt.Sprintf("%s/applications", s.config.ArgocdAPIURL)

	req, err := s.authService.CreateAuthenticatedRequest(ctx, "GET", url, nil)
	if err != nil {
		return types.ArgocdApplicationList{}, fmt.Errorf("failed to create authenticated request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return types.ArgocdApplicationList{}, fmt.Errorf("failed to execute request to ArgoCD: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return types.ArgocdApplicationList{}, fmt.Errorf("ArgoCD API returned status %d: %s", resp.StatusCode, string(body))
	}

	var appList types.ArgocdApplicationList
	if err := json.NewDecoder(resp.Body).Decode(&appList); err != nil {
		return types.ArgocdApplicationList{}, fmt.Errorf("failed to decode applications response: %w", err)
	}

	// Filter applications based on ignored projects
	var filteredApps []types.ArgocdApplication
	for _, app := range appList.Items {
		if !s.config.ShouldFilterProject(app.Spec.Project) {
			// Get ingress URLs for this application
			s.extractURLsFromApplication(&app)
			filteredApps = append(filteredApps, app)
		}
	}

	// Update the application list with filtered results
	appList.Items = filteredApps

	return appList, nil
}

// GetApplication retrieves a specific application from ArgoCD
func (s *ArgocdService) GetApplication(ctx context.Context, name string) (types.ArgocdApplication, error) {
	url := fmt.Sprintf("%s/applications/%s", s.config.ArgocdAPIURL, name)

	req, err := s.authService.CreateAuthenticatedRequest(ctx, "GET", url, nil)
	if err != nil {
		return types.ArgocdApplication{}, fmt.Errorf("failed to create authenticated request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return types.ArgocdApplication{}, fmt.Errorf("failed to execute request to ArgoCD: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return types.ArgocdApplication{}, fmt.Errorf("application '%s' not found", name)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return types.ArgocdApplication{}, fmt.Errorf("ArgoCD API returned status %d: %s", resp.StatusCode, string(body))
	}

	var app types.ArgocdApplication
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return types.ArgocdApplication{}, fmt.Errorf("failed to decode application response: %w", err)
	}

	// Check if the application's project should be filtered
	if s.config.ShouldFilterProject(app.Spec.Project) {
		return types.ArgocdApplication{}, fmt.Errorf("application '%s' belongs to filtered project '%s'", name, app.Spec.Project)
	}

	// Get ingress URLs for this application
	s.extractURLsFromApplication(&app)

	return app, nil
}

// ProxyRequest proxies a generic request to ArgoCD with authentication
func (s *ArgocdService) ProxyRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", s.config.ArgocdAPIURL, path)

	req, err := s.authService.CreateAuthenticatedRequest(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request to ArgoCD: %w", err)
	}

	return resp, nil
}

// GetProjectNames retrieves only the names of all projects (for grouping purposes)
func (s *ArgocdService) GetProjectNames(ctx context.Context) ([]string, error) {
	projects, err := s.GetProjects(ctx)
	if err != nil {
		return nil, err
	}

	var projectNames []string
	for _, project := range projects {
		projectNames = append(projectNames, project.Metadata.Name)
	}

	return projectNames, nil
}

// HealthCheck performs a health check by attempting to get projects
func (s *ArgocdService) HealthCheck(ctx context.Context) error {
	// Try to get a valid token first
	_, err := s.authService.GetValidToken(ctx)
	if err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	// Try to make a simple API call to verify connectivity
	url := fmt.Sprintf("%s/projects", s.config.ArgocdAPIURL)
	req, err := s.authService.CreateAuthenticatedRequest(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	// Use a shorter timeout for health checks
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req = req.WithContext(healthCtx)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}

// extractURLsFromApplication extracts external URLs from an application's status summary
func (s *ArgocdService) extractURLsFromApplication(app *types.ArgocdApplication) {
	var urls []string

	// Extract URLs from status summary if available
	if app.Status.Summary != nil && len(app.Status.Summary.ExternalURLs) > 0 {
		urls = append(urls, app.Status.Summary.ExternalURLs...)
	}

	// Set the ingress URLs on the application
	app.IngressURLs = urls
}

// ExtractIngressURLs extracts ingress URLs from application status summary
// This method is kept for interface compatibility
func (s *ArgocdService) ExtractIngressURLs(ctx context.Context, appName string) ([]string, error) {
	// Get the application first
	app, err := s.GetApplication(ctx, appName)
	if err != nil {
		return []string{}, err
	}

	// Return the URLs from the application
	return app.IngressURLs, nil
}

// GetApplicationsByGroup retrieves applications from a specific project group
func (s *ArgocdService) GetApplicationsByGroup(ctx context.Context, groupName string, cfg interface{}) (types.ArgocdApplicationList, error) {
	// Type assert the config to access project groups
	configObj, ok := cfg.(*config.Config)
	if !ok {
		return types.ArgocdApplicationList{}, fmt.Errorf("invalid config type")
	}

	// Find the specified group
	var targetGroup *config.ProjectGroup
	for i, group := range configObj.ProjectGroups {
		if group.Name == groupName {
			targetGroup = &configObj.ProjectGroups[i]
			break
		}
	}

	if targetGroup == nil {
		return types.ArgocdApplicationList{}, fmt.Errorf("project group '%s' not found", groupName)
	}

	// Get all applications
	allApplications, err := s.GetApplications(ctx)
	if err != nil {
		return types.ArgocdApplicationList{}, fmt.Errorf("failed to get applications: %w", err)
	}

	// Filter applications that belong to projects in this group
	var filteredApps []types.ArgocdApplication
	for _, app := range allApplications.Items {
		for _, projectName := range targetGroup.Projects {
			if app.Spec.Project == projectName {
				filteredApps = append(filteredApps, app)
				break
			}
		}
	}

	// Return filtered applications in the same format
	return types.ArgocdApplicationList{
		APIVersion: allApplications.APIVersion,
		Kind:       allApplications.Kind,
		Items:      filteredApps,
		Metadata:   allApplications.Metadata,
	}, nil
}

// GetApplicationsByProject retrieves applications from a specific project
func (s *ArgocdService) GetApplicationsByProject(ctx context.Context, projectName string) (types.ArgocdApplicationList, error) {
	// Get all applications
	allApplications, err := s.GetApplications(ctx)
	if err != nil {
		return types.ArgocdApplicationList{}, fmt.Errorf("failed to get applications: %w", err)
	}

	// Filter applications that belong to the specified project
	var filteredApps []types.ArgocdApplication
	for _, app := range allApplications.Items {
		if app.Spec.Project == projectName {
			filteredApps = append(filteredApps, app)
		}
	}

	// Return filtered applications in the same format
	return types.ArgocdApplicationList{
		APIVersion: allApplications.APIVersion,
		Kind:       allApplications.Kind,
		Items:      filteredApps,
		Metadata:   allApplications.Metadata,
	}, nil
}

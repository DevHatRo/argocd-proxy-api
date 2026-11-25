package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"argocd-proxy/cache"
	"argocd-proxy/config"
	"argocd-proxy/metrics"
	"argocd-proxy/types"
)

// ArgocdService provides access to ArgoCD API endpoints
type ArgocdService struct {
	config            *config.Config
	authService       types.AuthServiceInterface
	httpClient        *http.Client
	projectsCache     *cache.Cache[[]types.ArgocdProject]
	applicationsCache *cache.Cache[types.ArgocdApplicationList]
}

// statusError is a non-200 answer from ArgoCD
type statusError struct {
	code int
	body string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("ArgoCD API returned status %d: %s", e.code, e.body)
}

// NewArgocdService creates a new ArgoCD service instance
func NewArgocdService(cfg *config.Config, authSvc types.AuthServiceInterface) *ArgocdService {
	return &ArgocdService{
		config:      cfg,
		authService: authSvc,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		projectsCache:     cache.New[[]types.ArgocdProject](cfg.CacheTTL),
		applicationsCache: cache.New[types.ArgocdApplicationList](cfg.CacheTTL),
	}
}

// doInstrumented executes an HTTP request and records ArgoCD API metrics.
func (s *ArgocdService) doInstrumented(req *http.Request, endpoint string) (*http.Response, error) {
	start := time.Now()
	resp, err := s.httpClient.Do(req)
	duration := time.Since(start).Seconds()

	metrics.ArgocdAPIRequestDuration.WithLabelValues(endpoint).Observe(duration)

	status := "error"
	if err == nil {
		status = strconv.Itoa(resp.StatusCode)
	}
	metrics.ArgocdAPIRequestsTotal.WithLabelValues(endpoint, status).Inc()

	return resp, err
}

// getJSON performs an authenticated GET against ArgoCD and decodes the body into out
// (out may be nil). endpoint is the metrics label. A 401 means the cached token was
// revoked upstream, so the token is dropped and the call is retried once with a fresh one.
func (s *ArgocdService) getJSON(ctx context.Context, path, endpoint string, out interface{}) error {
	err := s.doGet(ctx, path, endpoint, out)

	var se *statusError
	if errors.As(err, &se) && se.code == http.StatusUnauthorized {
		s.authService.InvalidateToken()
		err = s.doGet(ctx, path, endpoint, out)
	}
	return err
}

func (s *ArgocdService) doGet(ctx context.Context, path, endpoint string, out interface{}) error {
	req, err := s.authService.CreateAuthenticatedRequest(ctx, http.MethodGet, s.config.ArgocdAPIURL+path, nil)
	if err != nil {
		return fmt.Errorf("failed to create authenticated request: %w", err)
	}

	resp, err := s.doInstrumented(req, endpoint)
	if err != nil {
		return fmt.Errorf("failed to execute request to ArgoCD: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return &statusError{code: resp.StatusCode, body: string(body)}
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode ArgoCD response: %w", err)
	}
	return nil
}

// GetProjects retrieves all projects from ArgoCD
func (s *ArgocdService) GetProjects(ctx context.Context) ([]types.ArgocdProject, error) {
	if cached, ok := s.projectsCache.Get(); ok {
		metrics.CacheHitsTotal.WithLabelValues("projects").Inc()
		return cached, nil
	}
	metrics.CacheMissesTotal.WithLabelValues("projects").Inc()

	var projectList types.ArgocdProjectList
	if err := s.getJSON(ctx, "/projects", "/projects", &projectList); err != nil {
		return nil, err
	}

	s.projectsCache.Set(projectList.Items)
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
	if cached, ok := s.applicationsCache.Get(); ok {
		metrics.CacheHitsTotal.WithLabelValues("applications").Inc()
		return cached, nil
	}
	metrics.CacheMissesTotal.WithLabelValues("applications").Inc()

	var appList types.ArgocdApplicationList
	if err := s.getJSON(ctx, "/applications", "/applications", &appList); err != nil {
		return types.ArgocdApplicationList{}, err
	}

	var filteredApps []types.ArgocdApplication
	for _, app := range appList.Items {
		if !s.config.ShouldFilterProject(app.Spec.Project) {
			setIngressURLs(&app)
			filteredApps = append(filteredApps, app)
		}
	}
	appList.Items = filteredApps

	s.applicationsCache.Set(appList)
	return appList, nil
}

// GetApplication retrieves a specific application from ArgoCD. An application that does
// not exist and one that belongs to a filtered project both return types.ErrNotFound,
// so callers cannot tell them apart.
func (s *ArgocdService) GetApplication(ctx context.Context, name string) (types.ArgocdApplication, error) {
	notFound := fmt.Errorf("application '%s': %w", name, types.ErrNotFound)
	if name == "." || name == ".." {
		return types.ArgocdApplication{}, notFound
	}

	var app types.ArgocdApplication
	err := s.getJSON(ctx, "/applications/"+url.PathEscape(name), "/applications/:name", &app)

	// ArgoCD answers 403 instead of 404 for an application that does not exist, so
	// that names cannot be probed. Both mean "not visible to this account".
	var se *statusError
	if errors.As(err, &se) && (se.code == http.StatusNotFound || se.code == http.StatusForbidden) {
		return types.ArgocdApplication{}, notFound
	}
	if err != nil {
		return types.ArgocdApplication{}, err
	}

	if s.config.ShouldFilterProject(app.Spec.Project) {
		return types.ArgocdApplication{}, notFound
	}

	setIngressURLs(&app)

	return app, nil
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

// HealthCheck verifies that ArgoCD answers an authenticated request
func (s *ArgocdService) HealthCheck(ctx context.Context) error {
	if err := s.getJSON(ctx, "/projects", "/healthcheck", nil); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	return nil
}

// setIngressURLs copies the external URLs from the status summary onto the application
func setIngressURLs(app *types.ArgocdApplication) {
	app.IngressURLs = nil
	if app.Status.Summary != nil {
		app.IngressURLs = append(app.IngressURLs, app.Status.Summary.ExternalURLs...)
	}
}

// applicationsInProjects returns the visible applications that belong to one of projects
func (s *ArgocdService) applicationsInProjects(ctx context.Context, projects ...string) (types.ArgocdApplicationList, error) {
	list, err := s.GetApplications(ctx)
	if err != nil {
		return types.ArgocdApplicationList{}, err
	}

	// Build a new slice: list.Items may be shared with the cache.
	var apps []types.ArgocdApplication
	for _, app := range list.Items {
		if slices.Contains(projects, app.Spec.Project) {
			apps = append(apps, app)
		}
	}
	list.Items = apps
	return list, nil
}

// GetApplicationsByGroup retrieves applications from a specific project group
func (s *ArgocdService) GetApplicationsByGroup(ctx context.Context, groupName string) (types.ArgocdApplicationList, error) {
	for _, group := range s.config.ProjectGroups {
		if group.Name == groupName {
			return s.applicationsInProjects(ctx, group.Projects...)
		}
	}
	return types.ArgocdApplicationList{}, fmt.Errorf("project group '%s': %w", groupName, types.ErrNotFound)
}

// GetApplicationsByProject retrieves applications from a specific project
func (s *ArgocdService) GetApplicationsByProject(ctx context.Context, projectName string) (types.ArgocdApplicationList, error) {
	return s.applicationsInProjects(ctx, projectName)
}

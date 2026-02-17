package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// ProjectGroup represents a group of projects with metadata
type ProjectGroup struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Projects    []string `json:"projects"`
}

// ProjectGroupsResponse represents the response for project groups endpoint
type ProjectGroupsResponse struct {
	Groups            []ProjectGroup `json:"groups"`
	UngroupedProjects []string       `json:"ungroupedProjects"`
}

// Config holds the application configuration
type Config struct {
	Port            string
	ArgocdAPIURL    string
	ArgocdUsername  string
	ArgocdPassword  string
	ProjectGroups   []ProjectGroup
	IgnoredProjects []string
	CacheTTL        time.Duration
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	config := &Config{
		Port:           getEnvOrDefault("PORT", "5001"),
		ArgocdAPIURL:   os.Getenv("ARGOCD_API_URL"),
		ArgocdUsername: os.Getenv("ARGOCD_USERNAME"),
		ArgocdPassword: os.Getenv("ARGOCD_PASSWORD"),
	}

	// Validate required environment variables
	if config.ArgocdAPIURL == "" {
		return nil, fmt.Errorf("ARGOCD_API_URL environment variable is required")
	}
	if config.ArgocdUsername == "" {
		return nil, fmt.Errorf("ARGOCD_USERNAME environment variable is required")
	}
	if config.ArgocdPassword == "" {
		return nil, fmt.Errorf("ARGOCD_PASSWORD environment variable is required")
	}

	// Load project groups from environment variable
	if projectGroupsJSON := os.Getenv("PROJECT_GROUPS"); projectGroupsJSON != "" {
		if err := json.Unmarshal([]byte(projectGroupsJSON), &config.ProjectGroups); err != nil {
			return nil, fmt.Errorf("failed to parse PROJECT_GROUPS: %w", err)
		}
	}

	// Load cache TTL from environment variable (default: 30s)
	cacheTTLStr := getEnvOrDefault("CACHE_TTL", "30s")
	cacheTTL, err := time.ParseDuration(cacheTTLStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CACHE_TTL %q: %w", cacheTTLStr, err)
	}
	config.CacheTTL = cacheTTL

	// Load ignored projects from environment variable
	if ignoredProjectsStr := os.Getenv("IGNORED_PROJECTS"); ignoredProjectsStr != "" {
		config.IgnoredProjects = strings.Split(ignoredProjectsStr, ",")
		// Trim whitespace from each project name
		for i, project := range config.IgnoredProjects {
			config.IgnoredProjects[i] = strings.TrimSpace(project)
		}
	}

	return config, nil
}

// getEnvOrDefault returns the value of an environment variable or a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// IsProjectIgnored checks if a project should be ignored based on pattern matching
// Supports exact match, prefix (*suffix), suffix (prefix*), and contains (*contains*)
func (c *Config) IsProjectIgnored(projectName string) bool {
	for _, ignored := range c.IgnoredProjects {
		if matchesPattern(projectName, ignored) {
			return true
		}
	}
	return false
}

// ShouldFilterProject checks if a project should be filtered out.
// Projects that are part of configured groups are never filtered, even if they match ignored patterns.
// Other projects are filtered based on the ignored projects patterns.
func (c *Config) ShouldFilterProject(projectName string) bool {
	// First check if this project is part of any configured group
	for _, group := range c.ProjectGroups {
		for _, groupProject := range group.Projects {
			if groupProject == projectName {
				// Project is part of a group, never filter it
				return false
			}
		}
	}

	// Project is not part of any group, check if it should be ignored
	return c.IsProjectIgnored(projectName)
}

// matchesPattern checks if a project name matches an ignore pattern
func matchesPattern(projectName, pattern string) bool {
	// Exact match
	if projectName == pattern {
		return true
	}

	// Single asterisk matches everything
	if pattern == "*" {
		return true
	}

	// Pattern with wildcards
	if strings.Contains(pattern, "*") {
		// Contains pattern (*text*)
		if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
			searchText := strings.Trim(pattern, "*")
			if searchText != "" {
				return strings.Contains(projectName, searchText)
			}
		}
		// Prefix pattern (text*)
		if strings.HasSuffix(pattern, "*") && !strings.HasPrefix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			return strings.HasPrefix(projectName, prefix)
		}
		// Suffix pattern (*text)
		if strings.HasPrefix(pattern, "*") && !strings.HasSuffix(pattern, "*") {
			suffix := strings.TrimPrefix(pattern, "*")
			return strings.HasSuffix(projectName, suffix)
		}
	}

	return false
}

// GetProjectGroups returns the configured project groups and ungrouped projects
func (c *Config) GetProjectGroups(allProjects []string) ProjectGroupsResponse {
	response := ProjectGroupsResponse{
		Groups: c.ProjectGroups,
	}

	// Create a map of all grouped projects
	groupedProjects := make(map[string]bool)
	for _, group := range c.ProjectGroups {
		for _, project := range group.Projects {
			groupedProjects[project] = true
		}
	}

	// Find ungrouped projects (excluding ignored ones)
	for _, project := range allProjects {
		if !groupedProjects[project] && !c.IsProjectIgnored(project) {
			response.UngroupedProjects = append(response.UngroupedProjects, project)
		}
	}

	return response
}

// FilterProjects returns a list of projects that are not ignored
func (c *Config) FilterProjects(projects []string) []string {
	var filtered []string
	for _, project := range projects {
		if !c.IsProjectIgnored(project) {
			filtered = append(filtered, project)
		}
	}
	return filtered
}

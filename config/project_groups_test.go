package config

import (
	"os"
	"reflect"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		wantErr  bool
		expected *Config
	}{
		{
			name: "valid config with all required vars",
			envVars: map[string]string{
				"PORT":             "8080",
				"ARGOCD_API_URL":   "https://argocd.example.com/api/v1",
				"ARGOCD_USERNAME":  "testuser",
				"ARGOCD_PASSWORD":  "testpass",
				"PROJECT_GROUPS":   `[{"name":"Frontend","description":"Frontend apps","projects":["web-app"]}]`,
				"IGNORED_PROJECTS": "test-*,*-dev",
			},
			wantErr: false,
			expected: &Config{
				Port:            "8080",
				ArgocdAPIURL:    "https://argocd.example.com/api/v1",
				ArgocdUsername:  "testuser",
				ArgocdPassword:  "testpass",
				ProjectGroups:   []ProjectGroup{{Name: "Frontend", Description: "Frontend apps", Projects: []string{"web-app"}}},
				IgnoredProjects: []string{"test-*", "*-dev"},
			},
		},
		{
			name: "valid config with defaults",
			envVars: map[string]string{
				"ARGOCD_API_URL":  "https://argocd.example.com/api/v1",
				"ARGOCD_USERNAME": "testuser",
				"ARGOCD_PASSWORD": "testpass",
			},
			wantErr: false,
			expected: &Config{
				Port:            "5001", // default port
				ArgocdAPIURL:    "https://argocd.example.com/api/v1",
				ArgocdUsername:  "testuser",
				ArgocdPassword:  "testpass",
				ProjectGroups:   nil,
				IgnoredProjects: nil,
			},
		},
		{
			name: "missing ARGOCD_API_URL",
			envVars: map[string]string{
				"ARGOCD_USERNAME": "testuser",
				"ARGOCD_PASSWORD": "testpass",
			},
			wantErr: true,
		},
		{
			name: "missing ARGOCD_USERNAME",
			envVars: map[string]string{
				"ARGOCD_API_URL":  "https://argocd.example.com/api/v1",
				"ARGOCD_PASSWORD": "testpass",
			},
			wantErr: true,
		},
		{
			name: "missing ARGOCD_PASSWORD",
			envVars: map[string]string{
				"ARGOCD_API_URL":  "https://argocd.example.com/api/v1",
				"ARGOCD_USERNAME": "testuser",
			},
			wantErr: true,
		},
		{
			name: "invalid PROJECT_GROUPS JSON",
			envVars: map[string]string{
				"ARGOCD_API_URL":  "https://argocd.example.com/api/v1",
				"ARGOCD_USERNAME": "testuser",
				"ARGOCD_PASSWORD": "testpass",
				"PROJECT_GROUPS":  `invalid json`,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all environment variables
			for _, env := range []string{"PORT", "ARGOCD_API_URL", "ARGOCD_USERNAME", "ARGOCD_PASSWORD", "PROJECT_GROUPS", "IGNORED_PROJECTS"} {
				os.Unsetenv(env)
			}

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			config, err := LoadConfig()

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadConfig() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("LoadConfig() unexpected error: %v", err)
				return
			}

			if config.Port != tt.expected.Port {
				t.Errorf("LoadConfig() Port = %v, want %v", config.Port, tt.expected.Port)
			}
			if config.ArgocdAPIURL != tt.expected.ArgocdAPIURL {
				t.Errorf("LoadConfig() ArgocdAPIURL = %v, want %v", config.ArgocdAPIURL, tt.expected.ArgocdAPIURL)
			}
			if config.ArgocdUsername != tt.expected.ArgocdUsername {
				t.Errorf("LoadConfig() ArgocdUsername = %v, want %v", config.ArgocdUsername, tt.expected.ArgocdUsername)
			}
			if config.ArgocdPassword != tt.expected.ArgocdPassword {
				t.Errorf("LoadConfig() ArgocdPassword = %v, want %v", config.ArgocdPassword, tt.expected.ArgocdPassword)
			}
			if !reflect.DeepEqual(config.ProjectGroups, tt.expected.ProjectGroups) {
				t.Errorf("LoadConfig() ProjectGroups = %v, want %v", config.ProjectGroups, tt.expected.ProjectGroups)
			}
			if !reflect.DeepEqual(config.IgnoredProjects, tt.expected.IgnoredProjects) {
				t.Errorf("LoadConfig() IgnoredProjects = %v, want %v", config.IgnoredProjects, tt.expected.IgnoredProjects)
			}
		})
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		pattern     string
		expected    bool
	}{
		// Exact matches
		{"exact match", "my-project", "my-project", true},
		{"exact no match", "my-project", "other-project", false},

		// Prefix patterns
		{"prefix match", "test-app", "test-*", true},
		{"prefix no match", "app-test", "test-*", false},
		{"prefix empty", "test-app", "*", true},

		// Suffix patterns
		{"suffix match", "app-dev", "*-dev", true},
		{"suffix no match", "dev-app", "*-dev", false},
		{"suffix empty", "app-dev", "*", true},

		// Contains patterns
		{"contains match", "my-test-app", "*test*", true},
		{"contains no match", "my-app", "*test*", false},
		{"contains at start", "test-app", "*test*", true},
		{"contains at end", "app-test", "*test*", true},

		// Edge cases
		{"empty pattern", "project", "", false},
		{"empty project", "", "pattern", false},
		{"both empty", "", "", true},
		{"just asterisk", "anything", "*", true},
		{"double asterisk start", "test-app", "**test", false}, // Should not match as it's not a valid contains pattern
		{"asterisk in middle", "test-app", "te*st", false},     // Should not match as it's not a supported pattern
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesPattern(tt.projectName, tt.pattern)
			if result != tt.expected {
				t.Errorf("matchesPattern(%q, %q) = %v, want %v", tt.projectName, tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestIsProjectIgnored(t *testing.T) {
	config := &Config{
		IgnoredProjects: []string{"test-*", "*-dev", "ignore-me", "*temp*"},
	}

	tests := []struct {
		name        string
		projectName string
		expected    bool
	}{
		{"prefix match", "test-app", true},
		{"suffix match", "app-dev", true},
		{"exact match", "ignore-me", true},
		{"contains match", "my-temp-app", true},
		{"no match", "production-app", false},
		{"partial match", "testing", false}, // Should not match "test-*"
		{"empty project", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.IsProjectIgnored(tt.projectName)
			if result != tt.expected {
				t.Errorf("IsProjectIgnored(%q) = %v, want %v", tt.projectName, result, tt.expected)
			}
		})
	}
}

func TestFilterProjects(t *testing.T) {
	config := &Config{
		IgnoredProjects: []string{"test-*", "*-dev"},
	}

	tests := []struct {
		name     string
		projects []string
		expected []string
	}{
		{
			name:     "filter some projects",
			projects: []string{"app1", "test-app", "app2-dev", "production"},
			expected: []string{"app1", "production"},
		},
		{
			name:     "filter no projects",
			projects: []string{"app1", "app2", "production"},
			expected: []string{"app1", "app2", "production"},
		},
		{
			name:     "filter all projects",
			projects: []string{"test-app", "app-dev"},
			expected: nil, // FilterProjects returns nil for empty result, not []string{}
		},
		{
			name:     "empty input",
			projects: []string{},
			expected: nil, // FilterProjects returns nil for empty result, not []string{}
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.FilterProjects(tt.projects)
			// Handle nil vs empty slice equivalence
			if len(result) == 0 && len(tt.expected) == 0 {
				return // Both are effectively empty
			}
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("FilterProjects(%v) = %v, want %v", tt.projects, result, tt.expected)
			}
		})
	}
}

func TestGetProjectGroups(t *testing.T) {
	config := &Config{
		ProjectGroups: []ProjectGroup{
			{Name: "Frontend", Description: "Frontend apps", Projects: []string{"web-app", "mobile-app"}},
			{Name: "Backend", Description: "Backend services", Projects: []string{"api-service"}},
		},
		IgnoredProjects: []string{"test-*"},
	}

	tests := []struct {
		name        string
		allProjects []string
		expected    ProjectGroupsResponse
	}{
		{
			name:        "with ungrouped projects",
			allProjects: []string{"web-app", "mobile-app", "api-service", "new-service", "test-app", "another-service"},
			expected: ProjectGroupsResponse{
				Groups: []ProjectGroup{
					{Name: "Frontend", Description: "Frontend apps", Projects: []string{"web-app", "mobile-app"}},
					{Name: "Backend", Description: "Backend services", Projects: []string{"api-service"}},
				},
				UngroupedProjects: []string{"new-service", "another-service"}, // test-app is ignored
			},
		},
		{
			name:        "no ungrouped projects",
			allProjects: []string{"web-app", "mobile-app", "api-service", "test-app"},
			expected: ProjectGroupsResponse{
				Groups: []ProjectGroup{
					{Name: "Frontend", Description: "Frontend apps", Projects: []string{"web-app", "mobile-app"}},
					{Name: "Backend", Description: "Backend services", Projects: []string{"api-service"}},
				},
				UngroupedProjects: nil, // test-app is ignored, others are grouped
			},
		},
		{
			name:        "all projects ungrouped",
			allProjects: []string{"service1", "service2"},
			expected: ProjectGroupsResponse{
				Groups: []ProjectGroup{
					{Name: "Frontend", Description: "Frontend apps", Projects: []string{"web-app", "mobile-app"}},
					{Name: "Backend", Description: "Backend services", Projects: []string{"api-service"}},
				},
				UngroupedProjects: []string{"service1", "service2"},
			},
		},
		{
			name:        "empty project list",
			allProjects: []string{},
			expected: ProjectGroupsResponse{
				Groups: []ProjectGroup{
					{Name: "Frontend", Description: "Frontend apps", Projects: []string{"web-app", "mobile-app"}},
					{Name: "Backend", Description: "Backend services", Projects: []string{"api-service"}},
				},
				UngroupedProjects: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.GetProjectGroups(tt.allProjects)

			if !reflect.DeepEqual(result.Groups, tt.expected.Groups) {
				t.Errorf("GetProjectGroups().Groups = %v, want %v", result.Groups, tt.expected.Groups)
			}

			// Handle nil vs empty slice equivalence for ungrouped projects
			if len(result.UngroupedProjects) == 0 && len(tt.expected.UngroupedProjects) == 0 {
				return // Both are effectively empty
			}
			if !reflect.DeepEqual(result.UngroupedProjects, tt.expected.UngroupedProjects) {
				t.Errorf("GetProjectGroups().UngroupedProjects = %v, want %v", result.UngroupedProjects, tt.expected.UngroupedProjects)
			}
		})
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "environment variable exists",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "env_value",
			expected:     "env_value",
		},
		{
			name:         "environment variable empty",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
		{
			name:         "environment variable not set",
			key:          "TEST_VAR_NOT_SET",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up environment
			os.Unsetenv(tt.key)

			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			result := getEnvOrDefault(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getEnvOrDefault(%q, %q) = %q, want %q", tt.key, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestShouldFilterProject(t *testing.T) {
	config := &Config{
		ProjectGroups: []ProjectGroup{
			{Name: "Frontend", Description: "Frontend apps", Projects: []string{"web-app", "test-frontend"}},
			{Name: "Backend", Description: "Backend services", Projects: []string{"api-service", "test-backend"}},
		},
		IgnoredProjects: []string{"test-*", "*-dev", "ignore-me"},
	}

	tests := []struct {
		name        string
		projectName string
		expected    bool
		description string
	}{
		// Projects in groups should NEVER be filtered, even if they match ignored patterns
		{
			name:        "grouped project matching ignored pattern",
			projectName: "test-frontend",
			expected:    false, // Should NOT be filtered despite matching "test-*"
			description: "Project in group should not be filtered even if it matches ignored pattern",
		},
		{
			name:        "grouped project matching ignored pattern 2",
			projectName: "test-backend",
			expected:    false, // Should NOT be filtered despite matching "test-*"
			description: "Another project in group should not be filtered even if it matches ignored pattern",
		},
		{
			name:        "grouped project not matching ignored pattern",
			projectName: "web-app",
			expected:    false, // Should NOT be filtered (in group and doesn't match pattern)
			description: "Project in group should not be filtered",
		},
		{
			name:        "grouped project not matching ignored pattern 2",
			projectName: "api-service",
			expected:    false, // Should NOT be filtered (in group and doesn't match pattern)
			description: "Another project in group should not be filtered",
		},

		// Projects NOT in groups should follow normal ignored pattern rules
		{
			name:        "ungrouped project matching ignored prefix",
			projectName: "test-app",
			expected:    true, // Should be filtered (matches "test-*")
			description: "Ungrouped project matching ignored pattern should be filtered",
		},
		{
			name:        "ungrouped project matching ignored suffix",
			projectName: "app-dev",
			expected:    true, // Should be filtered (matches "*-dev")
			description: "Ungrouped project matching ignored suffix should be filtered",
		},
		{
			name:        "ungrouped project matching exact ignored",
			projectName: "ignore-me",
			expected:    true, // Should be filtered (exact match)
			description: "Ungrouped project with exact ignored match should be filtered",
		},
		{
			name:        "ungrouped project not matching ignored patterns",
			projectName: "production",
			expected:    false, // Should NOT be filtered (doesn't match any pattern)
			description: "Ungrouped project not matching ignored patterns should not be filtered",
		},
		{
			name:        "ungrouped project not matching ignored patterns 2",
			projectName: "staging",
			expected:    false, // Should NOT be filtered (doesn't match any pattern)
			description: "Another ungrouped project not matching ignored patterns should not be filtered",
		},

		// Edge cases
		{
			name:        "empty project name",
			projectName: "",
			expected:    false, // Empty string doesn't match patterns and isn't in groups
			description: "Empty project name should not be filtered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ShouldFilterProject(tt.projectName)
			if result != tt.expected {
				t.Errorf("ShouldFilterProject(%q) = %v, want %v\nDescription: %s",
					tt.projectName, result, tt.expected, tt.description)
			}
		})
	}
}

func TestShouldFilterProjectWithNoGroups(t *testing.T) {
	// Test behavior when no project groups are configured
	config := &Config{
		ProjectGroups:   nil, // No groups configured
		IgnoredProjects: []string{"test-*", "*-dev"},
	}

	tests := []struct {
		name        string
		projectName string
		expected    bool
	}{
		{
			name:        "project matching ignored pattern with no groups",
			projectName: "test-app",
			expected:    true, // Should be filtered
		},
		{
			name:        "project not matching ignored pattern with no groups",
			projectName: "production",
			expected:    false, // Should not be filtered
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ShouldFilterProject(tt.projectName)
			if result != tt.expected {
				t.Errorf("ShouldFilterProject(%q) = %v, want %v", tt.projectName, result, tt.expected)
			}
		})
	}
}

func TestShouldFilterProjectWithNoIgnoredProjects(t *testing.T) {
	// Test behavior when no ignored projects are configured
	config := &Config{
		ProjectGroups: []ProjectGroup{
			{Name: "Frontend", Projects: []string{"web-app"}},
		},
		IgnoredProjects: nil, // No ignored projects
	}

	tests := []struct {
		name        string
		projectName string
		expected    bool
	}{
		{
			name:        "grouped project with no ignored patterns",
			projectName: "web-app",
			expected:    false, // Should not be filtered (in group)
		},
		{
			name:        "ungrouped project with no ignored patterns",
			projectName: "any-project",
			expected:    false, // Should not be filtered (no ignored patterns)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ShouldFilterProject(tt.projectName)
			if result != tt.expected {
				t.Errorf("ShouldFilterProject(%q) = %v, want %v", tt.projectName, result, tt.expected)
			}
		})
	}
}

// Benchmark tests for performance-critical functions
func BenchmarkMatchesPattern(b *testing.B) {
	patterns := []string{"test-*", "*-dev", "*temp*", "exact-match"}
	projectNames := []string{"test-app", "app-dev", "my-temp-app", "exact-match", "no-match"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, pattern := range patterns {
			for _, projectName := range projectNames {
				matchesPattern(projectName, pattern)
			}
		}
	}
}

func BenchmarkIsProjectIgnored(b *testing.B) {
	config := &Config{
		IgnoredProjects: []string{"test-*", "*-dev", "ignore-me", "*temp*", "another-*", "*-staging"},
	}
	projectNames := []string{"test-app", "app-dev", "my-temp-app", "production", "ignore-me", "no-match"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, projectName := range projectNames {
			config.IsProjectIgnored(projectName)
		}
	}
}

// TestUserScenarioFix demonstrates that the original user issue is resolved
func TestUserScenarioFix(t *testing.T) {
	// This test demonstrates the scenario where the user said:
	// "if I configure app like this, I will not be able to see any project or application,
	// not even the grouped ones, we need to exclude groups projects from ignored projects"

	config := &Config{
		ProjectGroups: []ProjectGroup{
			{
				Name:        "Frontend",
				Description: "Frontend applications",
				Projects:    []string{"test-frontend", "test-mobile"},
			},
			{
				Name:        "Backend",
				Description: "Backend services",
				Projects:    []string{"test-api", "test-auth"},
			},
		},
		IgnoredProjects: []string{"test-*"}, // This would normally filter ALL test-* projects
	}

	tests := []struct {
		name        string
		projectName string
		shouldShow  bool
		reason      string
	}{
		{
			name:        "grouped test project 1",
			projectName: "test-frontend",
			shouldShow:  true,
			reason:      "Project is in Frontend group, should be visible despite matching test-*",
		},
		{
			name:        "grouped test project 2",
			projectName: "test-mobile",
			shouldShow:  true,
			reason:      "Project is in Frontend group, should be visible despite matching test-*",
		},
		{
			name:        "grouped test project 3",
			projectName: "test-api",
			shouldShow:  true,
			reason:      "Project is in Backend group, should be visible despite matching test-*",
		},
		{
			name:        "grouped test project 4",
			projectName: "test-auth",
			shouldShow:  true,
			reason:      "Project is in Backend group, should be visible despite matching test-*",
		},
		{
			name:        "ungrouped test project",
			projectName: "test-random",
			shouldShow:  false,
			reason:      "Project is not in any group and matches test-*, should be filtered",
		},
		{
			name:        "ungrouped non-test project",
			projectName: "production",
			shouldShow:  true,
			reason:      "Project doesn't match test-* pattern, should be visible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldFilter := config.ShouldFilterProject(tt.projectName)
			actuallyShown := !shouldFilter

			if actuallyShown != tt.shouldShow {
				t.Errorf("Project '%s': expected shouldShow=%v, got=%v\nReason: %s",
					tt.projectName, tt.shouldShow, actuallyShown, tt.reason)
			}
		})
	}

	// Verify that the original user scenario now works:
	// All grouped projects should be visible even with "test-*" ignore pattern
	allProjectNames := []string{"test-frontend", "test-mobile", "test-api", "test-auth", "production", "test-random"}

	visibleProjects := []string{}
	for _, projectName := range allProjectNames {
		if !config.ShouldFilterProject(projectName) {
			visibleProjects = append(visibleProjects, projectName)
		}
	}

	expectedVisible := []string{"test-frontend", "test-mobile", "test-api", "test-auth", "production"}

	if len(visibleProjects) != len(expectedVisible) {
		t.Errorf("Expected %d visible projects, got %d\nExpected: %v\nActual: %v",
			len(expectedVisible), len(visibleProjects), expectedVisible, visibleProjects)
	}

	// Verify all grouped projects are visible
	for _, group := range config.ProjectGroups {
		for _, projectName := range group.Projects {
			if config.ShouldFilterProject(projectName) {
				t.Errorf("Grouped project '%s' in group '%s' should never be filtered", projectName, group.Name)
			}
		}
	}
}

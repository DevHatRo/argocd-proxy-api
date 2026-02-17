package types

import (
	"context"
	"net/http"
	"time"
)

// AuthServiceInterface defines the interface for authentication services
type AuthServiceInterface interface {
	GetValidToken(ctx context.Context) (string, error)
	CreateAuthenticatedRequest(ctx context.Context, method, url string, body []byte) (*http.Request, error)
	GetTokenStatus() map[string]interface{}
	StartTokenRefreshRoutine(ctx context.Context)
}

// ArgocdServiceInterface defines the interface for ArgoCD services
type ArgocdServiceInterface interface {
	GetProjects(ctx context.Context) ([]ArgocdProject, error)
	GetFilteredProjects(ctx context.Context) ([]ArgocdProject, error)
	GetApplications(ctx context.Context) (ArgocdApplicationList, error)
	GetApplication(ctx context.Context, name string) (ArgocdApplication, error)
	GetProjectNames(ctx context.Context) ([]string, error)
	HealthCheck(ctx context.Context) error
	ExtractIngressURLs(ctx context.Context, appName string) ([]string, error)
	GetApplicationsByGroup(ctx context.Context, groupName string, cfg interface{}) (ArgocdApplicationList, error)
	GetApplicationsByProject(ctx context.Context, projectName string) (ArgocdApplicationList, error)
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status      string                 `json:"status"`
	Timestamp   string                 `json:"timestamp"`
	Version     string                 `json:"version"`
	BuildTime   string                 `json:"buildTime"`
	TokenStatus map[string]interface{} `json:"tokenStatus"`
	ArgocdAPI   string                 `json:"argocdApiStatus"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code"`
}

// ArgocdSessionResponse represents the response from ArgoCD session endpoint
type ArgocdSessionResponse struct {
	Token string `json:"token"`
}

// ArgocdSessionRequest represents the request to ArgoCD session endpoint
type ArgocdSessionRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ArgocdProjectSpec represents the specification of an ArgoCD project
type ArgocdProjectSpec struct {
	SourceRepos  []string                   `json:"sourceRepos"`
	Destinations []ArgocdProjectDestination `json:"destinations"`
	Description  string                     `json:"description,omitempty"`
}

// ArgocdProjectDestination represents a destination in an ArgoCD project
type ArgocdProjectDestination struct {
	Namespace string `json:"namespace"`
	Server    string `json:"server"`
	Name      string `json:"name,omitempty"`
}

// ArgocdProjectStatus represents the status of an ArgoCD project
type ArgocdProjectStatus struct {
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// ArgocdProjectMetadata represents the metadata of an ArgoCD project
type ArgocdProjectMetadata struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	CreationTimestamp time.Time         `json:"creationTimestamp,omitempty"`
	UID               string            `json:"uid,omitempty"`
}

// ArgocdProject represents an ArgoCD project
type ArgocdProject struct {
	APIVersion string                `json:"apiVersion"`
	Kind       string                `json:"kind"`
	Metadata   ArgocdProjectMetadata `json:"metadata"`
	Spec       ArgocdProjectSpec     `json:"spec"`
	Status     ArgocdProjectStatus   `json:"status,omitempty"`
}

// ArgocdApplicationSource represents the source of an ArgoCD application
type ArgocdApplicationSource struct {
	RepoURL        string `json:"repoURL"`
	Path           string `json:"path,omitempty"`
	TargetRevision string `json:"targetRevision,omitempty"`
	Helm           *struct {
		ValueFiles []string `json:"valueFiles,omitempty"`
	} `json:"helm,omitempty"`
	Kustomize *struct {
		NamePrefix string `json:"namePrefix,omitempty"`
	} `json:"kustomize,omitempty"`
}

// ArgocdApplicationDestination represents the destination of an ArgoCD application
type ArgocdApplicationDestination struct {
	Server    string `json:"server"`
	Namespace string `json:"namespace"`
	Name      string `json:"name,omitempty"`
}

// ArgocdApplicationSpec represents the specification of an ArgoCD application
type ArgocdApplicationSpec struct {
	Project     string                       `json:"project"`
	Source      ArgocdApplicationSource      `json:"source"`
	Destination ArgocdApplicationDestination `json:"destination"`
	SyncPolicy  *struct {
		Automated *struct {
			Prune    bool `json:"prune,omitempty"`
			SelfHeal bool `json:"selfHeal,omitempty"`
		} `json:"automated,omitempty"`
	} `json:"syncPolicy,omitempty"`
}

// ArgocdApplicationHealth represents the health status of an ArgoCD application
type ArgocdApplicationHealth struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ArgocdApplicationSync represents the sync status of an ArgoCD application
type ArgocdApplicationSync struct {
	Status     string `json:"status"`
	ComparedTo *struct {
		Source      ArgocdApplicationSource      `json:"source"`
		Destination ArgocdApplicationDestination `json:"destination"`
	} `json:"comparedTo,omitempty"`
	Revision string `json:"revision,omitempty"`
}

// ArgocdApplicationStatus represents the status of an ArgoCD application
type ArgocdApplicationStatus struct {
	Health       ArgocdApplicationHealth   `json:"health"`
	Sync         ArgocdApplicationSync     `json:"sync"`
	Resources    []interface{}             `json:"resources,omitempty"`
	Conditions   []interface{}             `json:"conditions,omitempty"`
	ReconciledAt time.Time                 `json:"reconciledAt,omitempty"`
	Summary      *ArgocdApplicationSummary `json:"summary,omitempty"`
}

// ArgocdApplicationSummary represents the summary information of an ArgoCD application
type ArgocdApplicationSummary struct {
	ExternalURLs []string `json:"externalURLs,omitempty"`
	Images       []string `json:"images,omitempty"`
}

// ArgocdApplicationMetadata represents the metadata of an ArgoCD application
type ArgocdApplicationMetadata struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	CreationTimestamp time.Time         `json:"creationTimestamp,omitempty"`
	UID               string            `json:"uid,omitempty"`
}

// ArgocdApplication represents an ArgoCD application
type ArgocdApplication struct {
	APIVersion  string                    `json:"apiVersion"`
	Kind        string                    `json:"kind"`
	Metadata    ArgocdApplicationMetadata `json:"metadata"`
	Spec        ArgocdApplicationSpec     `json:"spec"`
	Status      ArgocdApplicationStatus   `json:"status,omitempty"`
	IngressURLs []string                  `json:"ingressUrls,omitempty"` // Enhanced with ingress URLs
}

// ArgocdApplicationList represents a list of ArgoCD applications
type ArgocdApplicationList struct {
	APIVersion string              `json:"apiVersion"`
	Kind       string              `json:"kind"`
	Items      []ArgocdApplication `json:"items"`
	Metadata   struct {
		ResourceVersion string `json:"resourceVersion,omitempty"`
	} `json:"metadata,omitempty"`
}

// ArgocdProjectList represents a list of ArgoCD projects
type ArgocdProjectList struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Items      []ArgocdProject `json:"items"`
	Metadata   struct {
		ResourceVersion string `json:"resourceVersion,omitempty"`
	} `json:"metadata,omitempty"`
}

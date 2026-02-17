// Copyright 2024 ArgoCD Proxy Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:generate go run github.com/swaggo/swag/cmd/swag@latest init

// @title ArgoCD Proxy API
// @version 1.0
// @description A proxy server for ArgoCD API with authentication and project filtering

// @host localhost:5001
// @BasePath /

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"argocd-proxy/auth"
	"argocd-proxy/config"
	_ "argocd-proxy/docs" // Import generated docs
	"argocd-proxy/services"
	"argocd-proxy/types"
)

// Version and build information, injected at build time via ldflags.
var (
	Version   = "dev"
	BuildTime = "unknown"
)

// Server holds the main server components
type Server struct {
	config        *config.Config
	authService   types.AuthServiceInterface
	argocdService types.ArgocdServiceInterface
	router        *gin.Engine
}

func main() {
	// Load environment variables from .env file (for development)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Set Gin mode
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create server instance
	server := &Server{
		config: cfg,
	}

	// Initialize services
	authSvc := auth.NewAuthService(cfg)
	server.authService = authSvc
	server.argocdService = services.NewArgocdService(cfg, authSvc)

	// Setup router and middleware
	server.setupRouter()

	// Start background token refresh routine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server.authService.StartTokenRefreshRoutine(ctx)

	// Start server with graceful shutdown
	server.start(ctx, cancel)
}

// setupRouter configures the Gin router with all routes and middleware
func (s *Server) setupRouter() {
	s.router = gin.New()

	// Add middleware
	s.router.Use(gin.Logger())
	s.router.Use(gin.Recovery())

	// CORS configuration
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowMethods = []string{"GET", "HEAD", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	corsConfig.ExposeHeaders = []string{"Content-Length"}
	s.router.Use(cors.New(corsConfig))

	// API routes (no prefix)
	s.router.GET("/health", s.healthCheck)
	s.router.GET("/project-groups", s.getProjectGroups)
	s.router.GET("/projects", s.getProjects)
	s.router.GET("/applications", s.getApplications)
	s.router.GET("/applications/:name", s.getApplication)
	s.router.GET("/groups/:group/applications", s.getApplicationsByGroup)
	s.router.GET("/projects/:project/applications", s.getApplicationsByProject)

	// Swagger documentation
	s.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Handle non-existent API routes
	s.router.NoRoute(s.handleNotFound)
}

// healthCheck handles the health check endpoint
// @Summary Health check
// @Description Get the health status of the ArgoCD proxy server
// @Tags health
// @Accept json
// @Produce json
// @Success 200 "Server is healthy"
// @Success 503 "Server is degraded"
// @Router /health [get]
func (s *Server) healthCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	response := types.HealthResponse{
		Status:      "healthy",
		Timestamp:   time.Now().Format(time.RFC3339),
		Version:     Version,
		BuildTime:   BuildTime,
		TokenStatus: s.authService.GetTokenStatus(),
		ArgocdAPI:   "unknown",
	}

	// Check ArgoCD API connectivity
	if err := s.argocdService.HealthCheck(ctx); err != nil {
		log.Printf("ArgoCD health check failed: %v", err)
		response.ArgocdAPI = fmt.Sprintf("error: %v", err)
		response.Status = "degraded"
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	response.ArgocdAPI = "healthy"
	c.JSON(http.StatusOK, response)
}

// getProjectGroups handles the project groups endpoint
// @Summary Get project groups
// @Description Get configured project groups and ungrouped projects from ArgoCD
// @Tags projects
// @Accept json
// @Produce json
// @Success 200 "Project groups response"
// @Failure 502 "Failed to retrieve projects from ArgoCD"
// @Router /project-groups [get]
func (s *Server) getProjectGroups(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Get all project names for grouping
	projectNames, err := s.argocdService.GetProjectNames(ctx)
	if err != nil {
		log.Printf("Failed to get project names: %v", err)
		s.errorResponse(c, http.StatusBadGateway, "Failed to retrieve projects from ArgoCD", err.Error())
		return
	}

	// Get project groups with ungrouped projects
	response := s.config.GetProjectGroups(projectNames)
	c.JSON(http.StatusOK, response)
}

// getProjects handles the projects endpoint (proxy to ArgoCD)
// @Summary Get filtered projects
// @Description Get projects from ArgoCD with filtering applied based on ignored projects configuration
// @Tags projects
// @Accept json
// @Produce json
// @Success 200 "Filtered projects list"
// @Failure 502 "Failed to retrieve projects from ArgoCD"
// @Router /projects [get]
func (s *Server) getProjects(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	projects, err := s.argocdService.GetFilteredProjects(ctx)
	if err != nil {
		log.Printf("Failed to get projects: %v", err)
		s.errorResponse(c, http.StatusBadGateway, "Failed to retrieve projects from ArgoCD", err.Error())
		return
	}

	// Return projects in the same format as ArgoCD
	response := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "List",
		"items":      projects,
	}

	c.JSON(http.StatusOK, response)
}

// getApplications handles the applications endpoint (proxy to ArgoCD with filtering)
// @Summary Get filtered applications
// @Description Get applications from ArgoCD with filtering applied based on ignored projects configuration
// @Tags applications
// @Accept json
// @Produce json
// @Success 200 "Filtered applications list"
// @Failure 502 "Failed to retrieve applications from ArgoCD"
// @Router /applications [get]
func (s *Server) getApplications(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	applications, err := s.argocdService.GetApplications(ctx)
	if err != nil {
		log.Printf("Failed to get applications: %v", err)
		s.errorResponse(c, http.StatusBadGateway, "Failed to retrieve applications from ArgoCD", err.Error())
		return
	}

	c.JSON(http.StatusOK, applications)
}

// getApplication handles the specific application endpoint (proxy to ArgoCD)
// @Summary Get specific application
// @Description Get a specific application by name from ArgoCD
// @Tags applications
// @Accept json
// @Produce json
// @Param name path string true "Application name"
// @Success 200 "Application details"
// @Failure 400 "Application name is required"
// @Failure 404 "Application not found"
// @Failure 502 "Failed to retrieve application from ArgoCD"
// @Router /applications/{name} [get]
func (s *Server) getApplication(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	appName := c.Param("name")
	if appName == "" {
		s.errorResponse(c, http.StatusBadRequest, "Application name is required", "")
		return
	}

	application, err := s.argocdService.GetApplication(ctx, appName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "ignored project") {
			s.errorResponse(c, http.StatusNotFound, fmt.Sprintf("Application '%s' not found", appName), err.Error())
			return
		}
		log.Printf("Failed to get application %s: %v", appName, err)
		s.errorResponse(c, http.StatusBadGateway, "Failed to retrieve application from ArgoCD", err.Error())
		return
	}

	c.JSON(http.StatusOK, application)
}

// getApplicationsByGroup handles getting applications from a specific project group
// @Summary Get applications by project group
// @Description Get all applications from a configured project group
// @Tags applications
// @Accept json
// @Produce json
// @Param group path string true "Project group name"
// @Success 200 "Applications from the specified group"
// @Failure 400 "Invalid group name"
// @Failure 404 "Project group not found"
// @Failure 502 "Failed to retrieve applications from ArgoCD"
// @Router /groups/{group}/applications [get]
func (s *Server) getApplicationsByGroup(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	groupName := c.Param("group")
	if groupName == "" {
		s.errorResponse(c, http.StatusBadRequest, "Group name is required", "")
		return
	}

	applications, err := s.argocdService.GetApplicationsByGroup(ctx, groupName, s.config)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.errorResponse(c, http.StatusNotFound, fmt.Sprintf("Project group '%s' not found", groupName), err.Error())
			return
		}
		log.Printf("Failed to get applications for group %s: %v", groupName, err)
		s.errorResponse(c, http.StatusBadGateway, "Failed to retrieve applications from ArgoCD", err.Error())
		return
	}

	c.JSON(http.StatusOK, applications)
}

// getApplicationsByProject handles getting applications from a specific project
// @Summary Get applications by project
// @Description Get all applications from a specific ArgoCD project
// @Tags applications
// @Accept json
// @Produce json
// @Param project path string true "Project name"
// @Success 200 "Applications from the specified project"
// @Failure 400 "Invalid project name"
// @Failure 502 "Failed to retrieve applications from ArgoCD"
// @Router /projects/{project}/applications [get]
func (s *Server) getApplicationsByProject(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	projectName := c.Param("project")
	if projectName == "" {
		s.errorResponse(c, http.StatusBadRequest, "Project name is required", "")
		return
	}

	applications, err := s.argocdService.GetApplicationsByProject(ctx, projectName)
	if err != nil {
		log.Printf("Failed to get applications for project %s: %v", projectName, err)
		s.errorResponse(c, http.StatusBadGateway, "Failed to retrieve applications from ArgoCD", err.Error())
		return
	}

	c.JSON(http.StatusOK, applications)
}

// handleNotFound handles 404 errors for non-existent routes
func (s *Server) handleNotFound(c *gin.Context) {
	// Check if the requested path is likely an API route (not swagger or static files)
	if !strings.HasPrefix(c.Request.URL.Path, "/swagger/") {
		s.errorResponse(c, http.StatusNotFound, "Endpoint not found", "")
		return
	}

	// For swagger routes, return a simple 404
	c.String(http.StatusNotFound, "404 page not found")
}

// errorResponse sends a standardized error response
func (s *Server) errorResponse(c *gin.Context, statusCode int, message, details string) {
	response := types.ErrorResponse{
		Error:   http.StatusText(statusCode),
		Message: message,
		Code:    statusCode,
	}

	if details != "" && gin.Mode() == gin.DebugMode {
		response.Message = fmt.Sprintf("%s: %s", message, details)
	}

	c.JSON(statusCode, response)
}

// start starts the HTTP server with graceful shutdown
func (s *Server) start(ctx context.Context, cancel context.CancelFunc) {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", s.config.Port),
		Handler: s.router,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting ArgoCD Proxy server on port %s", s.config.Port)
		log.Printf("Health check available at: http://localhost:%s/health", s.config.Port)
		log.Printf("Swagger documentation available at: http://localhost:%s/swagger/index.html", s.config.Port)
		log.Printf("ArgoCD API URL: %s", s.config.ArgocdAPIURL)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	cancel() // Cancel the context to stop background routines

	// Give outstanding requests 30 seconds to complete
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	} else {
		log.Println("Server exited gracefully")
	}
}

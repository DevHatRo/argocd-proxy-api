# argocd-proxy-api
ArgoCD Proxy API
# ArgoCD Proxy Server

A Go-based proxy server that provides authenticated access to ArgoCD API with project filtering and grouping capabilities. This server acts as a middleware between frontend applications and ArgoCD, handling authentication, token management, and project-based filtering.

## Features

- **ArgoCD Authentication**: Automatic token management with refresh before expiration
- **Project Filtering**: Filter applications based on configurable ignored projects
- **Project Grouping**: Group projects with metadata and show ungrouped projects
- **Static File Serving**: Serve React frontend applications
- **Health Monitoring**: Comprehensive health checks with token status
- **CORS Support**: Enable cross-origin requests for frontend applications
- **Graceful Shutdown**: Proper cleanup and graceful server shutdown
- **Docker Support**: Multi-stage Docker build with security best practices

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Server health check with token status |
| `/project-groups` | GET | Configured project groups and ungrouped projects |
| `/projects` | GET | Proxy to ArgoCD projects API (filtered) |
| `/applications` | GET | Proxy to ArgoCD applications API (filtered) |
| `/applications/:name` | GET | Proxy to specific application details |
| `/groups/:group/applications` | GET | Get all applications from a specific project group |
| `/projects/:project/applications` | GET | Get all applications from a specific project |
| `/swagger/*any` | GET | Swagger API documentation |

## Configuration

### Environment Variables

Copy `env.example` to `.env` and configure the following variables:

```bash
# Server Configuration
PORT=5001

# ArgoCD API Configuration
ARGOCD_API_URL=https://argocd.your-domain.com/api/v1
ARGOCD_USERNAME=your_argocd_username
ARGOCD_PASSWORD=your_argocd_password

# Project Groups Configuration (JSON format)
PROJECT_GROUPS=[{"name":"Frontend","description":"Frontend applications","projects":["web-app","mobile-app"]}]

# Ignored Projects Configuration (comma-separated with pattern support)
IGNORED_PROJECTS=test-*,*-dev,ignore-me
```

### Project Filtering Patterns

The `IGNORED_PROJECTS` variable supports pattern matching:

- **Exact match**: `project-name`
- **Prefix match**: `prefix*`
- **Suffix match**: `*suffix`
- **Contains match**: `*contains*`

Example: `IGNORED_PROJECTS=test-*,*-dev,ignore-me,*temp*`

#### Important: Project Groups Override Ignored Projects

**Projects explicitly configured in `PROJECT_GROUPS` are never filtered, even if they match `IGNORED_PROJECTS` patterns.**

For example, with this configuration:
```bash
PROJECT_GROUPS=[{"name":"Test","projects":["test-frontend","test-backend"]}]
IGNORED_PROJECTS=test-*
```

The projects `test-frontend` and `test-backend` will **not** be filtered despite matching the `test-*` pattern, because they are explicitly included in a project group. This allows you to have broad ignore patterns while still including specific projects you want to organize into groups.

### Project Groups Configuration

Configure project groups using JSON format in the `PROJECT_GROUPS` environment variable:

```json
[
  {
    "name": "Frontend",
    "description": "Frontend applications",
    "projects": ["web-app", "mobile-app", "landing-page"]
  },
  {
    "name": "Backend",
    "description": "Backend services",
    "projects": ["api-service", "auth-service", "notification-service"]
  }
]
```

## Quick Start

### Local Development

1. **Clone and setup**:
   ```bash
   git clone <repository>
   cd argocd-proxy
   cp env.example .env
   ```

2. **Configure environment**:
   Edit `.env` file with your ArgoCD credentials and settings.

3. **Install dependencies**:
   ```bash
   go mod download
   ```

4. **Generate Swagger documentation** (required for compilation):
   ```bash
   go run github.com/swaggo/swag/cmd/swag@latest init
   ```

5. **Run the server**:
   ```bash
   go run main.go
   ```

6. **Verify installation**:
   ```bash
   curl http://localhost:5001/health
   ```

### Docker Deployment

1. **Build the image**:
   ```bash
   docker build -t argocd-proxy .
   ```

2. **Run the container**:
   ```bash
   docker run -d \
     --name argocd-proxy \
     -p 5001:5001 \
     -e ARGOCD_API_URL=https://argocd.your-domain.com/api/v1 \
     -e ARGOCD_USERNAME=your_username \
     -e ARGOCD_PASSWORD=your_password \
     argocd-proxy
   ```

3. **Check health**:
   ```bash
   curl http://localhost:5001/api/health
   ```

### Docker Compose

Create a `docker-compose.yml` file:

```yaml
version: '3.8'
services:
  argocd-proxy:
    build: .
    ports:
      - "5001:5001"
    environment:
      - ARGOCD_API_URL=https://argocd.your-domain.com/api/v1
      - ARGOCD_USERNAME=your_username
      - ARGOCD_PASSWORD=your_password
      - PROJECT_GROUPS=[]
      - IGNORED_PROJECTS=test-*,*-dev
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:5001/api/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    restart: unless-stopped
```

Run with:
```bash
docker-compose up -d
```

## Enhanced Features

### Ingress URL Detection
The proxy automatically extracts and includes ingress URLs from ArgoCD applications:

- **Source**: Uses ArgoCD's built-in `status.summary.externalURLs` field
- **Performance**: No additional API calls required
- **Format**: URLs are provided in the `ingressUrls` array field
- **Availability**: Included in both `/applications` and `/applications/:name` responses

### Application Filtering by Group and Project
New endpoints for targeted application retrieval:

**By Project Group:**
- **Endpoint**: `GET /groups/:group/applications`
- **Purpose**: Get all applications from a configured project group
- **Example**: `/groups/Frontend/applications` returns all applications from projects in the "Frontend" group
- **Config**: Uses project groups defined in `PROJECT_GROUPS` environment variable

**By Individual Project:**
- **Endpoint**: `GET /projects/:project/applications`  
- **Purpose**: Get all applications from a specific ArgoCD project
- **Example**: `/projects/production/applications` returns all applications from the "production" project
- **Filter**: Automatically filters applications based on their `spec.project` field

**Benefits:**
- ✅ **Organized Access**: Get applications by logical groupings
- ✅ **Performance**: Efficient filtering on server side
- ✅ **Consistency**: Same response format as `/applications` endpoint
- ✅ **Flexibility**: Works with both configured groups and individual projects


## License

This project is licensed under the Apache License 2.0. See the [LICENSE](LICENSE) file for details.

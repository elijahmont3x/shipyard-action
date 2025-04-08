# Shipyard Action

A GitHub Action for deploying multi-application setups with persistent services, automated SSL, health checks, and more.

## Features

- **Multi-application support**: Deploy multiple apps in one action
- **Persistent services**: Support for databases, caches, and other services
- **Automated SSL handling**: Let's Encrypt integration with support for DNS challenges
- **Health checks**: Comprehensive health checks for all deployed applications
- **Dependency management**: Ensure services deploy in the correct order
- **Cleanup and rollback**: Automatic cleanup and rollback for failed deployments
- **Master proxy**: Route to multiple apps via subdomains or paths
- **Security scanning**: Scan container images for vulnerabilities
- **User-friendly configuration**: Simple YAML configuration file
- **External verification**: Validates that applications are accessible from the internet

## Usage

Add the following to your GitHub workflow file:

```yaml
name: Deploy

on:
  push:
    branches: [ main ]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Deploy with Shipyard
        uses: elijahmont3x/shipyard-action@master
        with:
          config: '.shipyard/config.yml'
          docker_host: 'unix:///var/run/docker.sock'
          log_level: 'info'
          dns_provider: 'cloudflare'
          dns_api_token: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          environment: 'production'
```

## Configuration

Create a `.shipyard/config.yml` file in your repository with the following structure:

```yaml
version: "1.0"
domain: "example.com"

ssl:
  enabled: true
  provider: "letsencrypt"
  email: "admin@example.com"
  selfSigned: false
  dnsChallenge: true
  dnsProvider: "cloudflare"
  dnsCredentials:
    CF_API_TOKEN: "${CLOUDFLARE_API_TOKEN}"

proxy:
  type: "nginx"  # or "traefik"
  port: 80
  httpsPort: 443

services:
  - name: "postgres"
    image: "postgres:13"
    environment:
      POSTGRES_USER: "app"
      POSTGRES_PASSWORD: "password"
      POSTGRES_DB: "app_db"
    ports:
      - "5432:5432"
    volumes:
      - source: "postgres_data"
        destination: "/var/lib/postgresql/data"
        type: "volume"
    healthCheck:
      type: "tcp"
      port: 5432
      interval: 10
      timeout: 5
      retries: 3
      startPeriod: 10

apps:
  - name: "backend"
    image: "myapp/backend:latest"
    subdomain: "api"
    environment:
      NODE_ENV: "production"
      DATABASE_URL: "postgres://app:password@postgres:5432/app_db"
    ports:
      - "8080:8080"
    dependsOn:
      - "postgres"
    healthCheck:
      type: "http"
      path: "/health"
      port: 8080
      interval: 10
      timeout: 5
      retries: 3
      startPeriod: 10

  - name: "frontend"
    image: "myapp/frontend:latest"
    subdomain: ""  # Use root domain
    environment:
      NODE_ENV: "production"
      API_URL: "https://api.example.com"
    ports:
      - "3000:3000"
    dependsOn:
      - "backend"
    healthCheck:
      type: "http"
      path: "/"
      port: 3000
```

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `config` | Path to the shipyard configuration file | Yes | `.shipyard/config.yml` |
| `docker_host` | Docker host URL | No | `unix:///var/run/docker.sock` |
| `log_level` | Log level (debug, info, warn, error) | No | `info` |
| `timeout` | Global timeout for deployment in minutes | No | `30` |
| `skip_security_scan` | Skip security scanning of container images | No | `false` |
| `dns_provider` | DNS provider for SSL certificate validation | No | `none` |
| `dns_api_token` | API token for DNS provider | No | - |
| `environment` | Deployment environment (development, staging, production) | No | `development` |

## Outputs

| Output | Description |
|--------|-------------|
| `deployed_apps` | JSON array of successfully deployed app names |
| `deployed_services` | JSON array of successfully deployed service names |
| `deployment_url` | Base URL for the deployment |

## Supported DNS Providers

For SSL certificate validation via DNS challenge, the following providers are supported:

- Cloudflare
- Route53
- DigitalOcean
- Google Cloud DNS
- Azure DNS
- And many more...

## FAQ

### Health Checks

#### How do health checks work for different types of applications?

- **Backend services**: These typically provide a dedicated health endpoint (e.g., `/health` or `/ping`) that checks database connections, cache availability, and other critical dependencies. These endpoints return a 200 OK status when everything is functioning correctly.

- **Static frontends**: For static web applications (served by Nginx, Apache, etc.), a health check to the root path (`/`) is usually sufficient. This checks that the web server is up and serving content correctly.

#### Why is checking the root path sufficient for static frontends?

When Shipyard performs a health check on a static frontend with `path: "/"`:

1. It makes an HTTP GET request to the container's web server
2. The web server serves the index.html file with a 200 OK response
3. This confirms the web server is running and properly configured to serve your content

This approach verifies exactly what a real user would experience when visiting your site, making it an ideal health check without requiring additional configuration.

#### What happens during a health check?

For HTTP health checks:
- After container startup, Shipyard waits for the configured `startPeriod`
- It then makes HTTP requests at the specified `interval` 
- Each request must return a 2xx status code within the `timeout` period
- If a check fails, it retries up to the configured `retries` count
- If all retries fail, the container is considered unhealthy

For TCP health checks:
- Similar to HTTP checks, but simply verifies that the specified port accepts connections

#### Do I need special files for health checks?

- For backend services: It's recommended to implement a lightweight health endpoint that checks critical dependencies
- For static frontends: No special files are needed - the existing index.html works perfectly
- For databases and caches: TCP health checks to the service port are usually sufficient

#### How does external verification work?

After a successful deployment, Shipyard automatically:

1. Constructs the public URLs for each application based on your domain, subdomain, and path configuration
2. Makes HTTP(S) requests to each URL to verify they're publicly accessible
3. Implements retry logic with exponential backoff to account for DNS propagation delays
4. Verifies both the main application URL and health check endpoints
5. Reports success or failure with detailed logs

This ensures that your applications are not just running in containers, but actually accessible to users on the internet.

## Development

### Prerequisites

- Go 1.20 or later
- Docker

### Building

```bash
go mod download
go build -o shipyard-action
```

### Testing

```bash
go test ./...
```

## License

MIT
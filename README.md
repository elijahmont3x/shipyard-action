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
        uses: elijahmont3x/shipyard-action@main
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
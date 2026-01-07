# Veneer Development Guide

This guide covers local development, testing, and contribution workflows for Veneer.

## Prerequisites

### Required Tools

- **make**: Build automation (usually pre-installed on macOS/Linux)
- **curl**: For downloading Go (usually pre-installed)
- **kubectl**: Kubernetes CLI configured with access to a cluster
- **git**: Version control

Go is installed automatically by `make` targets - no manual installation required.

### Optional Tools

- **kind**: For local Kubernetes testing
- **golangci-lint**: For full linting (currently disabled due to Go 1.24 compatibility)

### Required Infrastructure

Veneer requires:
1. **Kubernetes cluster** with Karpenter installed
2. **Lumina** deployed and exposing metrics via Prometheus
3. **Prometheus** server scraping Lumina metrics

## Quick Start

### 1. Clone and Setup

```bash
# Clone the repository
git clone https://github.com/Nextdoor/veneer.git
cd veneer

# Build (automatically installs Go and downloads dependencies)
make build

# Run tests
make test
```

The first run of `make build` or `make test` will:
1. Download and install Go into `./bin/go/` (version from `go.mod`)
2. Download Go module dependencies
3. Build or test the project

Subsequent runs skip the download if Go is already installed.

### 2. Configure Kubernetes Access

Ensure your kubeconfig is properly configured:

```bash
# Check current context
kubectl config current-context

# Verify cluster access
kubectl get nodes

# Verify Lumina is running (adjust namespace as needed)
kubectl get pods -n lumina-system
```

### 3. Port-Forward to Prometheus

Veneer needs to query Prometheus for Lumina metrics. Set up port-forwarding:

```bash
# Port-forward to Lumina's Prometheus instance
# (adjust namespace and service name as needed for your environment)
kubectl port-forward -n lumina-system svc/lumina-prometheus 9090:9090
```

**Keep this running in a separate terminal** while developing.

### 4. Run Veneer Locally

```bash
# In a new terminal, run the controller
make run
```

You should see output like:

```
2025-12-08T12:24:38-08:00	INFO	setup	loaded configuration	{"prometheus-url": "http://localhost:9090", "log-level": "debug"}
2025-12-08T12:24:38-08:00	INFO	setup	starting manager
2025-12-08T12:24:38-08:00	INFO	metrics-reconciler	Starting metrics reconciler	{"interval": "5m"}
```

### 5. Verify Operation

In another terminal, check that Veneer is working:

```bash
# Check health endpoint
curl http://localhost:8081/healthz

# Check metrics endpoint
curl http://localhost:8080/metrics

# Watch Veneer logs for reconciliation activity
# (logs appear in the terminal where you ran `make run`)
```

## Development Workflow

### Making Changes

1. **Create a feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes** following the project guidelines in [CLAUDE.md](CLAUDE.md)

3. **Run tests** (critical - see [Testing](#testing) section):
   ```bash
   make test
   ```

4. **Run linting**:
   ```bash
   make lint
   ```

5. **Commit your changes** using conventional commits:
   ```bash
   git add <files>
   git commit -m "feat(component): description of change"
   ```

### Conventional Commit Format

All commits must follow the conventional commits format:

```
<type>(component): <description>

[optional body]

[optional footer]
```

**Valid types**: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `ci`

**Examples**:
- `feat(overlay): add support for cross-account Savings Plans`
- `fix(prometheus): handle connection timeouts gracefully`
- `docs(readme): update installation instructions`
- `test(decision): add aggregation test cases`

### Pre-Commit Checklist

Before committing, **always** run:

```bash
# 1. Lint the code
make lint

# 2. Run all tests
make test

# 3. If both pass, commit
git add <files>
git commit -m "your message"
```

**Never skip these checks**. CI will fail if they don't pass.

## Testing

### Unit Tests

Veneer requires comprehensive test coverage. When adding new code, include tests in the same commit.

```bash
# Run all tests
make test

# View coverage report
make cover

# Run tests for a specific package (use local Go)
./bin/go/bin/go test ./pkg/overlay/... -v

# Run a specific test
./bin/go/bin/go test ./pkg/overlay/... -run TestAnalyzeComputeSavingsPlan -v

# Run with race detection
./bin/go/bin/go test -race ./pkg/... ./cmd/... ./internal/...
```

### Integration Tests

Integration tests validate end-to-end behavior with real Prometheus queries and K8s API interactions:

```bash
# Run all integration tests
./bin/go/bin/go test ./pkg/overlay/... -run Integration -v

# Run specific integration test
./bin/go/bin/go test ./pkg/overlay/... -run TestDecisionEngineIntegration -v
```

Integration tests use mock Prometheus servers and test fixtures, so they don't require a real cluster.

### Test-Driven Development (TDD)

We encourage TDD:

1. **Write a failing test** that describes the desired behavior
2. **Implement the feature** to make the test pass
3. **Refactor** if needed while keeping tests green

See [CLAUDE.md](CLAUDE.md) for detailed testing guidelines.

## Configuration

### Local Development Config

The `config.local.yaml` file is used for local development:

```yaml
# Prometheus URL (via kubectl port-forward)
prometheusUrl: "http://localhost:9090"

# Debug logging for development
logLevel: "debug"

# Bind addresses (defaults)
metricsBindAddress: ":8080"
healthProbeBindAddress: ":8081"

# Overlay management settings
overlayManagement:
  utilizationThreshold: 95.0  # Delete overlays at 95% utilization
  weights:
    reservedInstance: 30       # Highest priority
    ec2InstanceSavingsPlan: 20
    computeSavingsPlan: 10
```

### Environment Variables

You can override configuration via environment variables:

```bash
# Override Prometheus URL
export VENEER_PROMETHEUS_URL="http://prometheus.example.com:9090"

# Override log level
export VENEER_LOG_LEVEL="info"

# Run with overrides
make run
```

See [pkg/config/config.go](pkg/config/config.go) for all available environment variables.

## Debugging

### Enable Debug Logging

Edit `config.local.yaml`:

```yaml
logLevel: "debug"  # Shows detailed reconciliation logic
```

### Common Issues

#### Port 8081 Already in Use

```bash
# Find and kill the process using port 8081
lsof -ti:8081 | xargs kill -9

# Or change the port in config.local.yaml
healthProbeBindAddress: ":8082"
```

#### Prometheus Connection Failed

```bash
# Verify port-forward is running
lsof -i:9090

# Verify Prometheus is accessible
curl http://localhost:9090/api/v1/status/config

# Re-establish port-forward if needed
kubectl port-forward -n lumina-system svc/lumina-prometheus 9090:9090
```

#### "Context Deadline Exceeded" Errors

If you see timeout errors when querying Prometheus:

1. Check that Lumina is running and healthy
2. Verify Prometheus has scraped recent metrics
3. Check Prometheus query performance

```bash
# Check Lumina pods
kubectl get pods -n lumina-system

# Check Prometheus targets
curl http://localhost:9090/api/v1/targets
```

#### "No Matching Capacity" Warnings

If Veneer can't match Savings Plans utilization with capacity data:

1. Check that Lumina is exposing both metrics
2. Verify ARNs match between metrics
3. Enable debug logging to see the actual query results

## Project Structure

```
veneer/
├── cmd/
│   └── main.go              # Controller entrypoint
├── pkg/
│   ├── config/              # Configuration management
│   ├── overlay/             # Decision engine and overlay logic
│   ├── prometheus/          # Prometheus client for Lumina metrics
│   └── reconciler/          # Controller reconciliation loops
├── internal/
│   └── testutil/            # Test fixtures and mock servers
├── test/
│   └── e2e/                 # End-to-end tests (require real cluster)
├── charts/                  # Helm charts for deployment
├── config.local.yaml        # Local development configuration
└── config.example.yaml      # Example configuration
```

## Code Style Guidelines

See [CLAUDE.md](CLAUDE.md) for comprehensive style guidelines. Key points:

### Comments

- Explain **intent and reasoning**, not mechanics
- Document **why decisions were made**
- Call out **non-obvious implications**
- Reference RFC-0003 sections when relevant

### Code Coverage

- Strive for maximum code coverage (currently 86-94% across packages)
- Focus on testing valuable, real behavior
- Don't test pure data structures (field assignments)

### Open Source Readiness

**CRITICAL**: This project will be open-sourced. All code must:
- NOT contain internal references, URLs, or domain names
- NOT include internal service names or infrastructure details
- Use generic examples and placeholder values
- Be written as if already public

## Building and Deployment

### Build Binary

```bash
# Build locally
make build

# Binary will be at: bin/manager
./bin/manager --config=config.local.yaml
```

### Docker Image

```bash
# Build Docker image
make docker-build

# Push to registry
make docker-push
```

### Deploy to Cluster

```bash
# Install via Helm (adjust values as needed)
helm install veneer ./charts/veneer \
  --namespace veneer-system \
  --create-namespace \
  --set prometheusUrl=http://lumina-prometheus.lumina-system.svc:9090
```

## Contributing

### Pull Request Workflow

1. **Create a feature branch**: `git checkout -b feature/description`
2. **Make changes** following guidelines in [CLAUDE.md](CLAUDE.md)
3. **Add tests** for all new functionality
4. **Run pre-commit checks**: `make lint && make test`
5. **Commit** with conventional commit format
6. **Push branch**: `git push origin feature/description`
7. **Open Pull Request** in **draft mode** initially
8. **Update PR description** with:
   - Summary of changes
   - Testing performed
   - Related issues/tickets
9. **Mark ready for review** when CI passes

### Code Review Checklist

Before requesting review:

- [ ] No internal references or hardcoded data
- [ ] Comprehensive test coverage
- [ ] All tests pass locally
- [ ] `make lint` passes
- [ ] Code follows project conventions
- [ ] Documentation updated if needed
- [ ] Commit messages follow conventional format

### CI/CD Pipeline

Our CI pipeline enforces:

- All tests pass (unit + integration)
- Code formatting (gofmt, goimports)
- Go vet passes
- Build succeeds

Note: golangci-lint is currently disabled due to Go 1.24 compatibility issues. This will be re-enabled in a future update.

## Getting Help

- **Project Documentation**: See [README.md](README.md)
- **RFC-0003**: [Karpenter Cost-Aware Provisioning Design](https://github.com/Nextdoor/cloudeng/blob/main/rfcs/RFC-0003-karpenter-cost-aware-provisioning.md)
- **Development Guidelines**: See [CLAUDE.md](CLAUDE.md)
- **Issues**: [GitHub Issues](https://github.com/Nextdoor/veneer/issues)

## Troubleshooting Tips

### "No Such File or Directory" for config.local.yaml

The file should exist in the repo root. If missing:

```bash
# Verify you're in the repo root
pwd  # Should show: .../veneer

# Create from example
cp config.example.yaml config.local.yaml

# Edit prometheusUrl to use localhost:9090
vim config.local.yaml
```

### Tests Fail with "Connection Refused"

Integration tests use mock servers and shouldn't require real infrastructure. If you see connection errors:

1. Ensure you're running integration tests, not E2E tests
2. Check that no firewall is blocking localhost connections
3. Verify no other process is using the ports

### "Failed to Query Data Freshness"

This means Veneer can't reach Prometheus. Check:

1. Port-forward is running: `lsof -i:9090`
2. Prometheus is healthy: `curl http://localhost:9090/-/healthy`
3. Lumina metrics exist: `curl http://localhost:9090/api/v1/query?query=savings_plan_remaining_capacity`

### Building on Apple Silicon (M1/M2)

If you encounter CGO-related issues:

```bash
# Build with explicit architecture
GOARCH=arm64 make build

# Or use Rosetta for x86_64
arch -x86_64 make build
```

## Advanced Topics

### Custom Prometheus Queries

To add support for new Lumina metrics:

1. Update [pkg/prometheus/client.go](pkg/prometheus/client.go) with new query methods
2. Add corresponding types to handle the metric data
3. Add unit tests with mock Prometheus responses
4. Update integration tests to validate end-to-end behavior

### Adding New Decision Logic

To modify when overlays are created/deleted:

1. Update [pkg/overlay/decision.go](pkg/overlay/decision.go) decision engine
2. Update [pkg/config/config.go](pkg/config/config.go) if new configuration is needed
3. Add comprehensive unit tests for all decision paths
4. Add integration tests validating full workflow
5. Update configuration documentation

### Debugging Controller-Runtime

If you need to debug controller-runtime behavior:

```bash
# Enable verbose controller-runtime logging
export LOG_LEVEL=debug

# Run with additional flags
./bin/go/bin/go run ./cmd/main.go \
  --config=config.local.yaml \
  --zap-log-level=debug \
  --zap-devel=true
```

## Performance Considerations

### Metrics Reconciliation Interval

The default reconciliation interval is 5 minutes, matching Lumina's EC2 data refresh:

```yaml
# In config.local.yaml (if we add this setting)
metricsReconcileInterval: "5m"
```

### Prometheus Query Performance

If Prometheus queries are slow:

1. Check Prometheus resource usage
2. Verify query complexity (use Prometheus UI to test)
3. Consider adding indices or adjusting retention policies

### Memory Usage

Veneer's memory usage should be minimal (< 100MB typically). If higher:

1. Check for goroutine leaks: `curl http://localhost:8080/debug/pprof/goroutine`
2. Profile memory: `curl http://localhost:8080/debug/pprof/heap > heap.prof`
3. Analyze: `go tool pprof heap.prof`

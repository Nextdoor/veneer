---
title: "Development"
description: "Local development setup, testing, and contributing to Veneer."
weight: 50
---

## Prerequisites

### Required Tools

- **make**: Build automation (usually pre-installed on macOS/Linux)
- **curl**: For downloading Go (usually pre-installed)
- **kubectl**: Kubernetes CLI configured with access to a cluster
- **git**: Version control

Go is installed automatically by `make` targets -- no manual installation required.

### Optional Tools

- **kind**: For local Kubernetes testing
- **golangci-lint**: For full linting (currently disabled due to Go 1.24 compatibility)

## Quick Start

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

## Running Locally

### Against a Real Cluster

1. **Configure Kubernetes access**:
   ```bash
   kubectl config current-context
   kubectl get nodes
   kubectl get pods -n lumina-system
   ```

2. **Port-forward to Prometheus** (keep running in a separate terminal):
   ```bash
   kubectl port-forward -n lumina-system svc/lumina-prometheus 9090:9090
   ```

3. **Run the controller**:
   ```bash
   make run
   ```

4. **Verify**:
   ```bash
   curl http://localhost:8081/healthz
   curl http://localhost:8080/metrics
   ```

### Local Dev Environment (Fully Mocked)

For development without real AWS infrastructure, Veneer provides a fully mocked local environment using Kind, LocalStack, and Lumina with test data.

#### What's Included

| Component | Description |
|-----------|-------------|
| **Kind cluster** | Local Kubernetes cluster |
| **LocalStack** | Mocked AWS EC2/STS APIs with seeded instances |
| **Lumina** | Controller using test data for Savings Plans |
| **Prometheus** | Scrapes Lumina metrics |
| **Mock nodes** | K8s nodes correlated to LocalStack EC2 instances |

#### Setup

```bash
# One command to bring up everything
make dev-env-up
```

This creates:
- A Kind cluster named `veneer`
- 16 EC2 instances in LocalStack (m5.xlarge, c5.large, r5.large, m5.large)
- 3 Savings Plans with test rates
- 2 Reserved Instances
- Mock Kubernetes nodes with matching providerIDs

#### Available Make Targets

| Target | Description |
|--------|-------------|
| `make dev-env-up` | Deploy the full dev environment |
| `make dev-env-down` | Tear down the dev environment |
| `make dev-env-restart` | Restart (down + up) |
| `make dev-env-status` | Show status of all components |
| `make dev-env-logs` | Follow logs from all components |
| `make kind-create` | Create Kind cluster only |
| `make kind-delete` | Delete Kind cluster |

#### Running Against Local Environment

```bash
# Terminal 1: Start port-forward to Prometheus
kubectl port-forward -n prometheus svc/prometheus 9090:9090

# Terminal 2: Run Veneer
make run
```

#### Verifying the Environment

```bash
# Check all pods are running
make dev-env-status

# Check mock nodes exist with providerIDs
kubectl get nodes -l veneer.io/mock-node=true \
  -o custom-columns=NAME:.metadata.name,TYPE:.metadata.labels.node\\.kubernetes\\.io/instance-type,PROVIDER-ID:.spec.providerID

# Query Prometheus for Lumina metrics
kubectl port-forward -n prometheus svc/prometheus 9090:9090 &
curl -s 'http://localhost:9090/api/v1/query?query=savings_plan_remaining_capacity' | jq '.data.result'
```

#### Available Test Metrics

| Metric | Description | Example Values |
|--------|-------------|----------------|
| `ec2_instance_hourly_cost` | Per-instance hourly cost | $0.048 (m5.xlarge) |
| `savings_plan_remaining_capacity` | Unused SP capacity | $9.86, $4.71, $2.84 |
| `savings_plan_utilization_percent` | SP utilization rate | 1.4%, 5.8%, 5.3% |
| `savings_plan_hourly_commitment` | SP hourly commitment | $10, $5, $3 |
| `ec2_reserved_instance` | RI count by type | 2 (t2.small) |

#### EC2 Test Instances

| Instance Type | Count | Purpose |
|---------------|-------|---------|
| m5.xlarge | 6 | Tests EC2 Instance SP rates |
| c5.large | 4 | Tests EC2 Instance SP rates |
| r5.large | 4 | Tests Compute SP rates |
| m5.large (spot) | 2 | Tests spot pricing |

#### Customizing Test Data

1. **Add more EC2 instances**: Edit `hack/dev-env/localstack/init-configmap.yaml`
2. **Change Savings Plans**: Edit `hack/dev-env/lumina/configmap.yaml` under `testData.savingsPlans`
3. **Modify SP rates**: Edit `hack/dev-env/lumina/configmap.yaml` under `testData.savingsPlanRates`

After changes:
```bash
make dev-env-restart
```

#### Cleaning Up

```bash
# Remove dev environment but keep Kind cluster
make dev-env-down

# Remove everything including Kind cluster
make dev-env-down
make kind-delete
```

## Testing

### Unit Tests

```bash
# Run all tests
make test

# View coverage report
make cover

# Run tests for a specific package
./bin/go/bin/go test ./pkg/overlay/... -v

# Run a specific test
./bin/go/bin/go test ./pkg/overlay/... -run TestAnalyzeComputeSavingsPlan -v

# Run with race detection
./bin/go/bin/go test -race ./pkg/... ./cmd/... ./internal/...
```

### Integration Tests

Integration tests validate end-to-end behavior with mock Prometheus servers and K8s API interactions:

```bash
# Run all integration tests
./bin/go/bin/go test ./pkg/overlay/... -run Integration -v

# Run specific integration test
./bin/go/bin/go test ./pkg/overlay/... -run TestDecisionEngineIntegration -v
```

Integration tests use mock servers and test fixtures, so they don't require a real cluster.

### Test-Driven Development

We encourage TDD:

1. **Write a failing test** that describes the desired behavior
2. **Implement the feature** to make the test pass
3. **Refactor** if needed while keeping tests green

## Configuration

### Local Development Config

Create `config.local.yaml` from `config.example.yaml`:

```yaml
prometheusUrl: "http://localhost:9090"
logLevel: "debug"
metricsBindAddress: ":8080"
healthProbeBindAddress: ":8081"
aws:
  accountId: "123456789012"
  region: "us-west-2"
overlays:
  disabled: false
```

### Environment Variable Overrides

```bash
export VENEER_PROMETHEUS_URL="http://prometheus.example.com:9090"
export VENEER_LOG_LEVEL="info"
export VENEER_OVERLAY_DISABLED="true"
make run
```

### CLI Flags

```bash
./bin/manager --config=config.local.yaml --overlay-disabled
./bin/manager --help
```

## Project Structure

```
veneer/
├── cmd/
│   └── main.go              # Controller entrypoint
├── pkg/
│   ├── config/              # Configuration management
│   ├── metrics/             # Prometheus metrics instrumentation
│   ├── overlay/             # Decision engine and overlay generation
│   ├── preference/          # NodePool preference parsing and overlay generation
│   ├── prometheus/          # Prometheus client for Lumina metrics
│   └── reconciler/          # Controller reconciliation loops
├── internal/
│   └── testutil/            # Test fixtures and mock servers
├── test/
│   └── e2e/                 # End-to-end tests (require real cluster)
├── charts/                  # Helm charts for deployment
├── hack/
│   └── dev-env/             # Local development environment scripts
├── config.local.yaml        # Local development configuration
└── config.example.yaml      # Example configuration
```

## Building

```bash
# Build binary
make build
# Binary at: bin/manager

# Run directly
./bin/manager --config=config.local.yaml

# Build Docker image
make docker-build

# Push to registry
make docker-push
```

## Contributing

### Development Workflow

1. **Create a feature branch**: `git checkout -b feature/your-feature-name`
2. **Make changes** following the project guidelines
3. **Run tests**: `make test`
4. **Run linting**: `make lint`
5. **Commit** with conventional commits: `git commit -m "feat(component): description"`
6. **Push**: `git push origin feature/your-feature-name`
7. **Open a Pull Request** in draft mode

### Conventional Commit Format

```
<type>(component): <description>
```

**Valid types**: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `ci`

**Examples**:
- `feat(overlay): add support for cross-account Savings Plans`
- `fix(prometheus): handle connection timeouts gracefully`
- `test(decision): add aggregation test cases`

### Pre-Commit Checklist

Before every commit:

```bash
# 1. Lint the code
make lint

# 2. Run all tests
make test

# 3. If both pass, commit
git add <files>
git commit -m "your message"
```

### Code Review Checklist

- [ ] No internal references or hardcoded data
- [ ] Comprehensive test coverage for new functionality
- [ ] All tests pass locally
- [ ] `make lint` passes
- [ ] Code follows project conventions
- [ ] Documentation updated (if applicable)
- [ ] Commit messages follow conventional format

## Debugging

### Enable Debug Logging

```yaml
# config.local.yaml
logLevel: "debug"
```

### Controller-Runtime Debug Logging

```bash
./bin/go/bin/go run ./cmd/main.go \
  --config=config.local.yaml \
  --zap-log-level=debug \
  --zap-devel=true
```

### Performance Profiling

```bash
# Goroutine dump
curl http://localhost:8080/debug/pprof/goroutine

# Memory profile
curl http://localhost:8080/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

## Troubleshooting Development Issues

### "No Such File or Directory" for config.local.yaml

```bash
cp config.example.yaml config.local.yaml
# Edit prometheusUrl to use localhost:9090
```

### Tests Fail with "Connection Refused"

Integration tests use mock servers. If you see connection errors:
1. Ensure you're running integration tests, not E2E tests
2. Check that no firewall blocks localhost connections
3. Verify no other process uses the ports

### Building on Apple Silicon (M1/M2)

```bash
# Build with explicit architecture
GOARCH=arm64 make build

# Or use Rosetta for x86_64
arch -x86_64 make build
```

### Pods Stuck in Pending (Dev Environment)

```bash
kubectl describe pod -n lumina-system
# Mock nodes show as Unknown/NotReady - this is expected
# Only the control-plane node is real
```

### No Metrics in Prometheus (Dev Environment)

```bash
# Check Prometheus targets
kubectl port-forward -n prometheus svc/prometheus 9090:9090 &
curl -s 'http://localhost:9090/api/v1/targets' | jq '.data.activeTargets[] | {job: .labels.job, health: .health}'

# Check Lumina logs
kubectl logs -n lumina-system deployment/lumina-controller --tail=50
```

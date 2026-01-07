# Veneer

> Cost-aware Karpenter provisioning via NodeOverlay management

Veneer is a Kubernetes controller that optimizes Karpenter provisioning decisions by managing NodeOverlay resources based on real-time AWS Reserved Instance and Savings Plans data from [Lumina](https://github.com/Nextdoor/lumina).

## Status

**Phase 1: In Development** - Controller skeleton and Prometheus integration in progress.

See [RFC-0003](https://github.com/Nextdoor/cloudeng/blob/main/rfcs/RFC-0003-karpenter-cost-aware-provisioning.md) for full design.

## Overview

Veneer watches Lumina metrics and creates/updates/deletes Karpenter NodeOverlay CRs to:
- Prefer RI/SP-covered on-demand instances when cost-effective
- Fall back to spot when RI/SP capacity exhausted
- Avoid provisioning thrashing with smart debouncing

## Architecture

```
Lumina (RFC-0002) → Exposes SP/RI metrics to Prometheus
      ↓
Veneer (RFC-0003) → Queries metrics, manages NodeOverlays  
      ↓
Karpenter → Uses adjusted pricing for provisioning decisions
```

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md) for comprehensive development guide including:
- Local setup and prerequisites
- Running Veneer locally with kubectl port-forward
- Testing strategies (unit, integration, E2E)
- Debugging tips and troubleshooting
- Code style guidelines and contribution workflow

### Quick Start

```bash
# Build (auto-installs Go into ./bin/go/)
make build

# Run tests
make test

# Lint
make lint
```

No Go installation required - `make` handles it automatically.

### Running Locally

To run Veneer locally against a Kubernetes cluster:

1. **Port-forward to Prometheus/Lumina:**
   ```bash
   kubectl port-forward -n lumina-system svc/lumina-prometheus 9090:9090
   ```

2. **Run the controller:**
   ```bash
   make run
   ```

See [DEVELOPMENT.md](DEVELOPMENT.md) for detailed instructions, troubleshooting, and advanced topics

## Configuration

See [config.example.yaml](config.example.yaml) for all configuration options.

Local development uses `config.local.yaml`:

```yaml
prometheusUrl: "http://localhost:9090"  # Via port-forward
logLevel: "debug"                        # Verbose logging for development
```

## Contributing

This project will be open-sourced. See [DEVELOPMENT.md](DEVELOPMENT.md) for the full development guide and [CLAUDE.md](CLAUDE.md) for code style guidelines.

**Key Requirements:**
- Comprehensive test coverage with focus on integration tests
- All code must be open-source ready (no internal references)
- Follow conventional commit format
- Run `make lint` and `make test` before committing

## License

Apache 2.0 (to be confirmed)

## Credits

Built by the Platform Engineering team as a companion to [Lumina](https://github.com/Nextdoor/lumina).

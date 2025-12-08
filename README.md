# Karve

> Cost-aware Karpenter provisioning via NodeOverlay management

Karve is a Kubernetes controller that optimizes Karpenter provisioning decisions by managing NodeOverlay resources based on real-time AWS Reserved Instance and Savings Plans data from [Lumina](https://github.com/Nextdoor/lumina).

## Status

**Phase 1: In Development** - Controller skeleton and Prometheus integration in progress.

See [RFC-0003](https://github.com/Nextdoor/cloudeng/blob/main/rfcs/RFC-0003-karpenter-cost-aware-provisioning.md) for full design.

## Overview

Karve watches Lumina metrics and creates/updates/deletes Karpenter NodeOverlay CRs to:
- Prefer RI/SP-covered on-demand instances when cost-effective
- Fall back to spot when RI/SP capacity exhausted
- Avoid provisioning thrashing with smart debouncing

## Architecture

```
Lumina (RFC-0002) → Exposes SP/RI metrics to Prometheus
      ↓
Karve (RFC-0003) → Queries metrics, manages NodeOverlays  
      ↓
Karpenter → Uses adjusted pricing for provisioning decisions
```

## Development

### Quick Start

```bash
# Build
make build

# Run tests
make test

# Lint
make lint
```

### Running Locally

To run Karve locally against a Kubernetes cluster:

1. **Port-forward to Prometheus/Lumina:**
   ```bash
   kubectl port-forward -n lumina-system svc/lumina-prometheus 9090:9090
   ```

2. **Configure kubeconfig:**
   Ensure `~/.kube/config` points to your target cluster

3. **Run the controller:**
   ```bash
   make run
   ```

The `make run` target uses `config.local.yaml` which is pre-configured for local development with `http://localhost:9090`.

**Troubleshooting:**
- If port 8081 is in use: `lsof -ti:8081 | xargs kill -9`
- If config not found: `config.local.yaml` should already exist (created by repo)
- If Prometheus connection fails: Verify port-forward is running

## Configuration

See [config.example.yaml](config.example.yaml) for all configuration options.

Local development uses `config.local.yaml`:

```yaml
prometheusUrl: "http://localhost:9090"  # Via port-forward
logLevel: "debug"                        # Verbose logging for development
```

## Contributing

This project will be open-sourced. See [CLAUDE.md](CLAUDE.md) for development guidelines.

**Requirements:**
- 100% code coverage
- Integration tests for all functionality
- No internal references

## License

Apache 2.0 (to be confirmed)

## Credits

Built by the Platform Engineering team as a companion to [Lumina](https://github.com/Nextdoor/lumina).

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

```bash
# Build
make build

# Run tests
make test

# Run locally (requires kubeconfig)
make run
```

## Configuration

Create `config.yaml`:

```yaml
prometheusUrl: "http://prometheus:9090"
logLevel: "info"
```

See [config.example.yaml](config.example.yaml) for all options.

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

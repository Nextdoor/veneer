# Veneer

> Cost-aware Karpenter provisioning via NodeOverlay management

<!-- TODO: Add CI/release badges here -->

Veneer is a Kubernetes controller that optimizes Karpenter provisioning decisions by managing NodeOverlay resources based on real-time AWS Reserved Instance and Savings Plans data from [Lumina](https://github.com/Nextdoor/lumina). It automatically prefers RI/SP-covered on-demand instances when cost-effective and falls back to spot when capacity is exhausted.

**Full documentation: <https://oss.nextdoor.com/veneer/docs/>**

## Install

```bash
helm install veneer ./charts/veneer \
  --namespace veneer-system \
  --create-namespace \
  --set prometheusUrl=http://lumina-prometheus.lumina-system.svc:9090
```

## Documentation

| Topic | Link |
|-------|------|
| Getting Started | [Quick Start Guide](https://oss.nextdoor.com/veneer/docs/getting-started/) |
| Concepts | [Architecture & Concepts](https://oss.nextdoor.com/veneer/docs/concepts/) |
| Instance Selection | [How Karpenter Selects Instances](https://oss.nextdoor.com/veneer/docs/concepts/instance-selection/) |
| Bin-Packing | [Bin-Packing & NodeOverlay](https://oss.nextdoor.com/veneer/docs/concepts/binpacking/) |
| Configuration | [Configuration Reference](https://oss.nextdoor.com/veneer/docs/configuration/) |
| Instance Preferences | [NodePool Preference Annotations](https://oss.nextdoor.com/veneer/docs/configuration/preferences/) |
| Development | [Development Guide](https://oss.nextdoor.com/veneer/docs/development/) |

## Quick Start (Development)

```bash
make build    # Auto-installs Go into ./bin/go/
make test     # Run tests
make lint     # Lint
```

See the [Development Guide](https://oss.nextdoor.com/veneer/docs/development/) for full instructions.

## Contributing

See the [Development Guide](https://oss.nextdoor.com/veneer/docs/development/) for setup, testing, code style, and contribution workflow.

## License

Apache 2.0

## Credits

Built by the Nextdoor Cloud Engineering team as a companion to [Lumina](https://github.com/Nextdoor/lumina).

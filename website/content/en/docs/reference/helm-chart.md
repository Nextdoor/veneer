---
title: "Helm Chart"
description: "Veneer Helm chart values reference."
weight: 30
---

The Veneer Helm chart deploys the controller to a Kubernetes cluster. This page documents all available Helm values.

## Installation

```bash
helm install veneer veneer/veneer \
  --namespace veneer-system \
  --create-namespace \
  -f values.yaml
```

## Values Reference

### Replica and Image

| Value | Default | Description |
|-------|---------|-------------|
| `replicaCount` | `2` | Number of controller replicas (leader election handles HA) |
| `image.repository` | `ghcr.io/nextdoor/veneer` | Container image repository |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `image.tag` | `""` | Image tag (defaults to chart `appVersion`) |
| `imagePullSecrets` | `[]` | Image pull secrets for private registries |

### Naming

| Value | Default | Description |
|-------|---------|-------------|
| `nameOverride` | `""` | Override the name of the chart |
| `fullnameOverride` | `""` | Override the full name of the release |

### Service Account

| Value | Default | Description |
|-------|---------|-------------|
| `serviceAccount.create` | `true` | Create a service account |
| `serviceAccount.automount` | `true` | Automatically mount API credentials |
| `serviceAccount.annotations` | `{}` | Annotations (e.g., `eks.amazonaws.com/role-arn` for IRSA) |
| `serviceAccount.name` | `""` | Service account name (auto-generated if empty) |

### Pod Configuration

| Value | Default | Description |
|-------|---------|-------------|
| `podAnnotations` | `{}` | Annotations to add to the pod |
| `podLabels` | `{}` | Labels to add to the pod |

### Security Context

The chart enforces a restrictive security posture by default:

| Value | Default | Description |
|-------|---------|-------------|
| `podSecurityContext.runAsNonRoot` | `true` | Run as non-root user |
| `podSecurityContext.runAsUser` | `65532` | User ID |
| `podSecurityContext.fsGroup` | `65532` | Filesystem group ID |
| `podSecurityContext.seccompProfile.type` | `RuntimeDefault` | Seccomp profile |
| `securityContext.allowPrivilegeEscalation` | `false` | Prevent privilege escalation |
| `securityContext.capabilities.drop` | `["ALL"]` | Drop all capabilities |
| `securityContext.readOnlyRootFilesystem` | `true` | Read-only root filesystem |
| `securityContext.runAsNonRoot` | `true` | Run as non-root |
| `securityContext.runAsUser` | `65532` | User ID |
| `securityContext.seccompProfile.type` | `RuntimeDefault` | Seccomp profile |

### Controller Configuration

The `config` section is passed directly to Veneer's `config.yaml`. See [Configuration Reference]({{< relref "configuration" >}}) for full details.

| Value | Default | Description |
|-------|---------|-------------|
| `config.prometheusUrl` | `"http://prometheus:9090"` | Prometheus URL for Lumina metrics |
| `config.logLevel` | `"info"` | Log level (`debug`, `info`, `warn`, `error`) |
| `config.metricsBindAddress` | `":8080"` | Metrics endpoint bind address |
| `config.healthProbeBindAddress` | `":8081"` | Health probe bind address |
| `config.aws.accountId` | `"123456789012"` | AWS account ID (**required**, change this) |
| `config.aws.region` | `"us-west-2"` | AWS region (**required**) |
| `config.overlays.utilizationThreshold` | `95.0` | SP utilization threshold for overlay deletion |
| `config.overlays.weights.reservedInstance` | `30` | RI overlay weight |
| `config.overlays.weights.ec2InstanceSavingsPlan` | `20` | EC2 Instance SP overlay weight |
| `config.overlays.weights.computeSavingsPlan` | `10` | Compute SP overlay weight |
| `config.overlays.naming.reservedInstancePrefix` | `"cost-aware-ri"` | RI overlay name prefix |
| `config.overlays.naming.ec2InstanceSavingsPlanPrefix` | `"cost-aware-ec2-sp"` | EC2 Instance SP overlay name prefix |
| `config.overlays.naming.computeSavingsPlanPrefix` | `"cost-aware-compute-sp"` | Compute SP overlay name prefix |

### Controller Manager

| Value | Default | Description |
|-------|---------|-------------|
| `controllerManager.leaderElection.enabled` | `true` | Enable leader election for HA |
| `controllerManager.extraArgs` | `[]` | Extra CLI arguments for the controller |

### Metrics Service

| Value | Default | Description |
|-------|---------|-------------|
| `metricsService.type` | `ClusterIP` | Service type |
| `metricsService.port` | `8080` | Service port |
| `metricsService.annotations` | `{}` | Service annotations |

### Resources

| Value | Default | Description |
|-------|---------|-------------|
| `resources.limits.cpu` | `"1"` | CPU limit |
| `resources.limits.memory` | `512Mi` | Memory limit |
| `resources.requests.cpu` | `200m` | CPU request |
| `resources.requests.memory` | `128Mi` | Memory request |

### Health Probes

| Value | Default | Description |
|-------|---------|-------------|
| `livenessProbe.httpGet.path` | `/healthz` | Liveness probe path |
| `livenessProbe.httpGet.port` | `8081` | Liveness probe port |
| `livenessProbe.initialDelaySeconds` | `15` | Initial delay |
| `livenessProbe.periodSeconds` | `20` | Check interval |
| `livenessProbe.timeoutSeconds` | `1` | Timeout |
| `livenessProbe.failureThreshold` | `3` | Failures before restart |
| `readinessProbe.httpGet.path` | `/readyz` | Readiness probe path |
| `readinessProbe.httpGet.port` | `8081` | Readiness probe port |
| `readinessProbe.initialDelaySeconds` | `5` | Initial delay |
| `readinessProbe.periodSeconds` | `10` | Check interval |
| `readinessProbe.timeoutSeconds` | `1` | Timeout |
| `readinessProbe.failureThreshold` | `3` | Failures before unready |

### Volumes

| Value | Default | Description |
|-------|---------|-------------|
| `volumes` | `[]` | Additional volumes for the deployment |
| `volumeMounts` | `[]` | Additional volume mounts |

### Scheduling

| Value | Default | Description |
|-------|---------|-------------|
| `nodeSelector` | `{}` | Node selector for pod assignment |
| `tolerations` | `[]` | Tolerations for pod assignment |
| `affinity` | `{}` | Affinity rules for pod assignment |

### RBAC

| Value | Default | Description |
|-------|---------|-------------|
| `rbac.create` | `true` | Create ClusterRole and ClusterRoleBinding |

### ServiceMonitor

For Prometheus Operator integration:

| Value | Default | Description |
|-------|---------|-------------|
| `serviceMonitor.enabled` | `true` | Create a ServiceMonitor resource |
| `serviceMonitor.interval` | `30s` | Scrape interval |
| `serviceMonitor.scrapeTimeout` | `10s` | Scrape timeout |
| `serviceMonitor.labels` | `{}` | Additional labels |
| `serviceMonitor.annotations` | `{}` | Additional annotations |
| `serviceMonitor.relabelings` | `[]` | Relabel configurations |
| `serviceMonitor.metricRelabelings` | `[]` | Metric relabel configurations |

### Lumina Subchart

Veneer can optionally deploy Lumina as a subchart:

| Value | Default | Description |
|-------|---------|-------------|
| `lumina.enabled` | `false` | Enable Lumina as a subchart |

When enabled, all Lumina chart values can be passed under the `lumina` key. See [Lumina documentation](https://github.com/Nextdoor/lumina) for available values.

## Example: Production Values

```yaml
replicaCount: 2

config:
  prometheusUrl: "http://lumina-prometheus.lumina-system.svc:9090"
  logLevel: "info"
  aws:
    accountId: "123456789012"
    region: "us-west-2"
  overlays:
    utilizationThreshold: 95.0

resources:
  limits:
    cpu: "1"
    memory: 512Mi
  requests:
    cpu: 200m
    memory: 128Mi

serviceMonitor:
  enabled: true
  interval: 30s

serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: "arn:aws:iam::123456789012:role/veneer-controller"
```

## Example: Development Values

```yaml
replicaCount: 1

config:
  prometheusUrl: "http://prometheus.prometheus.svc:9090"
  logLevel: "debug"
  aws:
    accountId: "000000000000"
    region: "us-west-2"
  overlays:
    disabled: true  # Dry-run mode

controllerManager:
  leaderElection:
    enabled: false  # Single replica, no need for leader election

resources:
  limits:
    cpu: 500m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 64Mi
```

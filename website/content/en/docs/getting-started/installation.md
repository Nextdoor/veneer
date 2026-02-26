---
title: "Installation"
description: "Prerequisites, Helm installation, and verification for Veneer."
weight: 10
---

## Prerequisites

Veneer requires the following components in your Kubernetes cluster:

| Component | Version | Purpose |
|-----------|---------|---------|
| **Karpenter** | v1.0+ (with NodeOverlay support) | Node provisioning with NodeOverlay CRD |
| **Lumina** | Latest | Exposes RI/SP metrics to Prometheus |
| **Prometheus** | Any | Scrapes and stores Lumina metrics |

### Karpenter with NodeOverlay Support

Veneer manages [NodeOverlay]({{< relref "../reference/nodeoverlay" >}}) custom resources, which are part of the Karpenter `v1alpha1` API group. Ensure your Karpenter installation includes the NodeOverlay CRD:

```bash
kubectl get crd nodeoverlays.karpenter.sh
```

### Lumina

[Lumina](https://github.com/Nextdoor/lumina) must be deployed and actively exposing Savings Plans and Reserved Instance metrics. Verify Lumina is running:

```bash
kubectl get pods -n lumina-system
```

### Prometheus

A Prometheus server must be scraping Lumina metrics. Veneer queries Prometheus for:

- `savings_plan_remaining_capacity` -- SP remaining capacity in dollars/hour
- `savings_plan_utilization_percent` -- SP utilization percentage
- `ec2_reserved_instance` -- Reserved Instance counts by type

Verify metrics are available:

```bash
curl -s 'http://<prometheus-url>:9090/api/v1/query?query=savings_plan_remaining_capacity' | jq '.data.result | length'
```

## Helm Installation

### Add the Helm Repository

```bash
helm repo add veneer https://nextdoor.github.io/veneer
helm repo update
```

### Install Veneer

```bash
helm install veneer veneer/veneer \
  --namespace veneer-system \
  --create-namespace \
  --set config.prometheusUrl=http://lumina-prometheus.lumina-system.svc:9090 \
  --set config.aws.accountId=123456789012 \
  --set config.aws.region=us-west-2
```

{{% pageinfo color="warning" %}}
The `config.aws.accountId` and `config.aws.region` values are **required**. Veneer uses them to scope Prometheus queries to only return capacity data relevant to this cluster.
{{% /pageinfo %}}

### Install from Local Chart

If installing from the repository source:

```bash
helm install veneer ./charts/veneer \
  --namespace veneer-system \
  --create-namespace \
  --set config.prometheusUrl=http://lumina-prometheus.lumina-system.svc:9090 \
  --set config.aws.accountId=123456789012 \
  --set config.aws.region=us-west-2
```

### Custom Values File

For production deployments, create a `values.yaml`:

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
    weights:
      reservedInstance: 30
      ec2InstanceSavingsPlan: 20
      computeSavingsPlan: 10

serviceMonitor:
  enabled: true
```

```bash
helm install veneer veneer/veneer \
  --namespace veneer-system \
  --create-namespace \
  -f values.yaml
```

See the [Helm Chart Reference]({{< relref "../reference/helm-chart" >}}) for all available values.

## Verification

### Check Pod Status

```bash
kubectl get pods -n veneer-system
```

You should see the Veneer controller pods running (2 replicas by default with leader election):

```
NAME                      READY   STATUS    RESTARTS   AGE
veneer-6b8f9c4d7f-abc12   1/1     Running   0          2m
veneer-6b8f9c4d7f-def34   1/1     Running   0          2m
```

### Check Health Endpoints

```bash
# Port-forward to the controller
kubectl port-forward -n veneer-system svc/veneer 8081:8081

# Check health
curl http://localhost:8081/healthz
# Expected: ok

# Check readiness
curl http://localhost:8081/readyz
# Expected: ok
```

### Check Metrics

```bash
kubectl port-forward -n veneer-system svc/veneer-metrics 8080:8080
curl -s http://localhost:8080/metrics | grep veneer_
```

You should see metrics including `veneer_info`, `veneer_lumina_data_available`, and others. See [Metrics Reference]({{< relref "../reference/metrics" >}}) for the full catalog.

### Verify NodeOverlay Creation

After Veneer has run at least one reconciliation cycle (default 5 minutes), check for NodeOverlays:

```bash
kubectl get nodeoverlays -l app.kubernetes.io/managed-by=veneer
```

If Lumina reports available Savings Plans or Reserved Instance capacity, you should see overlays created with names like `cost-aware-ri-*`, `cost-aware-ec2-sp-*`, or `cost-aware-compute-sp-*`.

### Check Logs

```bash
kubectl logs -n veneer-system -l app.kubernetes.io/name=veneer --tail=50
```

Look for log lines indicating successful reconciliation:

```
INFO    metrics-reconciler    Starting metrics reconciler    {"interval": "5m"}
INFO    metrics-reconciler    Reconciliation complete        {"overlays-created": 3, "overlays-deleted": 0}
```

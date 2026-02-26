---
title: "Documentation"
description: "Comprehensive documentation for the Veneer cost-aware Karpenter provisioning controller."
weight: 20
---

Veneer is a Kubernetes controller that optimizes [Karpenter](https://karpenter.sh/) provisioning decisions by managing NodeOverlay resources based on real-time AWS Reserved Instance and Savings Plans data from [Lumina](https://github.com/Nextdoor/lumina).

## How It Works

```mermaid
flowchart TD
    subgraph AWS["AWS"]
        direction LR
        RI["Reserved Instances"]
        SP["Savings Plans"]
        EC2["EC2 Instances"]
    end

    Lumina["Lumina — Cost Data Controller"]

    Prom["Prometheus"]

    Veneer["Veneer — Overlay Controller"]

    NO["NodeOverlays"]

    Karpenter["Karpenter"]

    Nodes["Cost-Optimized EC2 Nodes"]

    RI & SP & EC2 --> Lumina
    Lumina -->|"expose RI/SP cost metrics"| Prom
    Prom -->|"query instance cost data"| Veneer
    Veneer -->|"create & update overlays"| NO
    NO -->|"adjust instance pricing & priority"| Karpenter
    Karpenter -->|"provision nodes"| Nodes

    style AWS fill:#fff3e0,stroke:#e65100,color:#e65100
    style Lumina fill:#e3f2fd,stroke:#1565c0,color:#1565c0
    style Prom fill:#fbe9e7,stroke:#bf360c,color:#bf360c
    style Veneer fill:#e0f2f1,stroke:#00695c,color:#00695c
    style NO fill:#f1f8e9,stroke:#33691e,color:#33691e
    style Karpenter fill:#ede7f6,stroke:#4527a0,color:#4527a0
    style Nodes fill:#f5f5f5,stroke:#616161,color:#616161
    style RI fill:#fff3e0,stroke:#e65100,color:#e65100
    style SP fill:#fff3e0,stroke:#e65100,color:#e65100
    style EC2 fill:#fff3e0,stroke:#e65100,color:#e65100
```

Veneer continuously watches Lumina's cost metrics and creates/updates/deletes Karpenter **NodeOverlay** CRs to prefer RI/SP-covered on-demand instances when cost-effective, fall back to spot when capacity is exhausted, avoid provisioning thrashing with smart debouncing, and express instance preferences via NodePool annotations.

Use the section navigation below to explore the documentation.

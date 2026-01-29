# Karpenter Bin-Packing: How It Affects NodeOverlay Effectiveness

This document explains how Karpenter's bin-packing algorithm can affect—and sometimes bypass—NodeOverlay price adjustments, leading to unexpected instance selection behavior.

## Table of Contents

- [Overview](#overview)
- [The Bin-Packing Algorithm](#the-bin-packing-algorithm)
- [How Bin-Packing Filters Instance Types](#how-bin-packing-filters-instance-types)
- [The ARM64 Size Gap Problem](#the-arm64-size-gap-problem)
- [When NodeOverlay Cannot Help](#when-nodeoverlay-cannot-help)
- [Diagnosing the Issue](#diagnosing-the-issue)
- [Solutions and Workarounds](#solutions-and-workarounds)

---

## Overview

Veneer's NodeOverlay feature influences instance selection by adjusting prices, which become Priority values in AWS CreateFleet requests. However, this influence only works when multiple instance types are eligible candidates.

**The key insight**: Karpenter's bin-packing algorithm filters instance types *before* NodeOverlay can influence selection. If bin-packing eliminates all instances of a particular architecture, NodeOverlay has nothing to prefer.

```mermaid
flowchart LR
    subgraph Karpenter["Karpenter Processing"]
        Pods["Pending Pods"] --> BinPack["Bin-Packing<br/>Algorithm"]
        BinPack --> Filter["Filter Instance<br/>Types"]
        Filter --> Overlay["Apply NodeOverlay<br/>Price Adjustments"]
        Overlay --> Fleet["CreateFleet<br/>Request"]
    end

    Filter -->|"If all ARM64<br/>filtered out"| NoInfluence["NodeOverlay<br/>cannot help"]

    style NoInfluence fill:#FFB6C1
```

---

## The Bin-Packing Algorithm

Karpenter uses a **First-Fit Decreasing (FFD)** bin-packing algorithm to minimize the number of nodes needed for pending pods.

### How It Works

1. **Sort pods by resource requirements** (largest first)
2. **For each pod**, try to fit it into an existing "virtual node"
3. **If no existing node can fit the pod**, create a new virtual node
4. **Select the smallest instance type** that can satisfy each virtual node's aggregate requirements

### Example: Bin-Packing in Action

Consider 10 pending pods, each requesting 12 vCPUs:

```
Total CPU needed: 10 pods × 12 vCPU = 120 vCPU
```

Karpenter's options:
- **Option A**: 2 nodes × 64 vCPU = 128 vCPU capacity (8 vCPU wasted)
- **Option B**: 1 node × 128 vCPU = 128 vCPU capacity (8 vCPU wasted)

The algorithm prefers **Option B** because it minimizes node count, even though the total capacity is the same.

---

## How Bin-Packing Filters Instance Types

When Karpenter determines that a single node with 120+ vCPU is optimal, it filters the instance type list to only include types that can satisfy this requirement.

```mermaid
flowchart TB
    subgraph Before["Before Bin-Packing Filter"]
        All["All Instance Types"]
        ARM64_1["arm64: 24xlarge (96 vCPU)"]
        ARM64_2["arm64: 48xlarge (192 vCPU)"]
        x86_1["x86: 24xlarge (96 vCPU)"]
        x86_2["x86: 32xlarge (128 vCPU)"]
        x86_3["x86: 48xlarge (192 vCPU)"]
        All --> ARM64_1 & ARM64_2 & x86_1 & x86_2 & x86_3
    end

    subgraph Filter["Bin-Pack Requirement: ≥120 vCPU"]
        Check{"Can fit<br/>120 vCPU?"}
    end

    subgraph After["After Bin-Packing Filter"]
        Remaining["Eligible Instance Types"]
        ARM64_2_ok["arm64: 48xlarge (192 vCPU)"]
        x86_2_ok["x86: 32xlarge (128 vCPU)"]
        x86_3_ok["x86: 48xlarge (192 vCPU)"]
        Remaining --> ARM64_2_ok & x86_2_ok & x86_3_ok
    end

    Before --> Filter --> After

    ARM64_1 -.->|"96 < 120 ❌"| Filter
    x86_1 -.->|"96 < 120 ❌"| Filter

    style ARM64_1 fill:#FFB6C1
    style x86_1 fill:#FFB6C1
```

In this example, both architectures still have eligible instances (48xlarge), so NodeOverlay can influence the selection.

---

## The ARM64 Size Gap Problem

AWS Graviton (ARM64) instances have a size gap that x86 instances don't have:

| Size | ARM64 (Graviton) | x86 (Intel/AMD) |
|------|------------------|-----------------|
| 24xlarge | 96 vCPU | 96 vCPU |
| 32xlarge | **Does not exist** | 128 vCPU |
| 48xlarge | 192 vCPU | 192 vCPU |

This gap creates a range (97-128 vCPU) where only x86 instances are available.

### The Problem Scenario

If your NodePool has `maxVcpu: 128` and bin-packing requires 100+ vCPU:

```mermaid
flowchart TB
    subgraph NodePool["NodePool Configuration"]
        Config["maxVcpu: 128"]
    end

    subgraph Available["Available Instance Types"]
        ARM64["ARM64 Options"]
        x86["x86 Options"]

        ARM64_24["24xlarge: 96 vCPU"]
        ARM64_48["48xlarge: 192 vCPU ❌<br/>(exceeds maxVcpu)"]

        x86_24["24xlarge: 96 vCPU"]
        x86_32["32xlarge: 128 vCPU ✓"]

        ARM64 --> ARM64_24 & ARM64_48
        x86 --> x86_24 & x86_32
    end

    subgraph BinPack["Bin-Packing: Need 100 vCPU"]
        Need["Minimum: 100 vCPU"]
    end

    subgraph Result["Eligible After Filtering"]
        Only["Only x86 32xlarge qualifies"]
    end

    NodePool --> Available
    Available --> BinPack
    BinPack --> Result

    style ARM64_24 fill:#FFB6C1
    style ARM64_48 fill:#FFB6C1
    style x86_24 fill:#FFB6C1
    style Only fill:#FFB6C1
```

**Result**: ARM64 is completely filtered out. NodeOverlay's `-50%` price adjustment on ARM64 has no effect because there are no ARM64 candidates.

---

## When NodeOverlay Cannot Help

NodeOverlay adjusts the *priority* of instance types in the CreateFleet request. It cannot:

1. **Add instance types** that were filtered out by bin-packing
2. **Change NodePool requirements** (like maxVcpu)
3. **Override Karpenter's bin-packing decisions**

### The Decision Flow

```mermaid
flowchart TB
    subgraph Phase1["Phase 1: Karpenter Filtering"]
        direction TB
        P1_1["NodePool requirements filter"]
        P1_2["Bin-packing size requirements"]
        P1_3["AMI compatibility filter"]
        P1_1 --> P1_2 --> P1_3
    end

    subgraph Phase2["Phase 2: NodeOverlay Influence"]
        direction TB
        P2_1["Apply price adjustments"]
        P2_2["Set Priority values"]
        P2_3["Choose allocation strategy"]
    end

    subgraph Phase3["Phase 3: AWS Selection"]
        direction TB
        P3_1["Check spot capacity"]
        P3_2["Apply allocation strategy"]
        P3_3["Select instance"]
    end

    Phase1 -->|"Filtered list"| Phase2
    Phase2 -->|"CreateFleet request"| Phase3

    Note1["NodeOverlay can only<br/>influence instances that<br/>survive Phase 1"]

    Phase2 -.-> Note1

    style Note1 fill:#FFFACD
```

---

## Diagnosing the Issue

### Symptom: x86 Selected Despite ARM64 Price Preference

If you've configured a NodeOverlay to prefer ARM64 but are still seeing x86 instances:

1. **Check the NodeClaim requirements**
   ```bash
   kubectl get nodeclaim <name> -o yaml | grep -A 50 requirements
   ```

   Look for the instance type list. If only x86 types are listed, bin-packing has already filtered out ARM64.

2. **Check CloudTrail CreateFleet requests**

   Look at the `LaunchTemplateConfigs` in the request:
   - **Two configs** (ARM64 AMI + x86 AMI) = NodeOverlay can influence
   - **One config** (x86 AMI only) = ARM64 was filtered out before NodeOverlay

3. **Check the aggregate CPU requirements**

   Sum up the CPU requests of pods that triggered the NodeClaim. If it exceeds the ARM64 size threshold (e.g., 96 vCPU for 24xlarge), that's likely the cause.

### Example: Identifying the Problem

```bash
# Get the NodeClaim
kubectl get nodeclaim example-abc123 -o yaml
```

```yaml
spec:
  requirements:
    - key: node.kubernetes.io/instance-type
      operator: In
      values:
        - m7a.32xlarge    # 128 vCPU, x86 only
        - m7i.32xlarge    # 128 vCPU, x86 only
        - r7a.32xlarge    # 128 vCPU, x86 only
        # Notice: No ARM64 instances listed!
```

This NodeClaim has already been filtered to only include 32xlarge x86 instances.

---

## Solutions and Workarounds

### Solution 1: Increase maxVcpu to Include Larger ARM64 Sizes

If your NodePool has `maxVcpu: 128`, increase it to `192` to allow Graviton 48xlarge:

```yaml
requirements:
  - key: karpenter.k8s.aws/instance-cpu
    operator: Lt
    values:
      - "193"  # Allows up to 192 vCPU (48xlarge)
```

**Trade-off**: Larger nodes mean more pods per node, which may affect blast radius during node failures.

### Solution 2: Reduce maxVcpu to Exclude x86-Only Sizes

Set `maxVcpu: 96` to prevent bin-packing from choosing 32xlarge:

```yaml
requirements:
  - key: karpenter.k8s.aws/instance-cpu
    operator: Lt
    values:
      - "97"  # Max 96 vCPU (24xlarge)
```

**Trade-off**: Karpenter may create more nodes to fit the same workload.

### Solution 3: Explicitly Exclude 32xlarge Sizes

```yaml
requirements:
  - key: karpenter.k8s.aws/instance-size
    operator: NotIn
    values:
      - 32xlarge
```

**Trade-off**: Same as Solution 2—more nodes may be created.

### Solution 4: Force Architecture in NodePool

If ARM64 is strongly preferred, constrain the NodePool:

```yaml
requirements:
  - key: kubernetes.io/arch
    operator: In
    values:
      - arm64
```

**Trade-off**: No x86 fallback if ARM64 spot capacity is unavailable.

---

## Summary

| Stage | What Happens | Can NodeOverlay Influence? |
|-------|--------------|---------------------------|
| **NodePool Requirements** | Filter by CPU, memory, family, etc. | No |
| **Bin-Packing** | Determine minimum node size needed | No |
| **AMI Mapping** | Group instance types by architecture | No |
| **Price Adjustment** | Apply NodeOverlay adjustments | **Yes** |
| **CreateFleet** | AWS selects from eligible instances | **Yes** (via Priority) |

**Key Takeaways**:

1. NodeOverlay influences selection *among eligible candidates*, not the filtering process
2. The ARM64 size gap (no 32xlarge Graviton) can eliminate ARM64 from consideration
3. Check NodeClaim requirements and CloudTrail to diagnose unexpected selections
4. Adjust `maxVcpu` or exclude specific sizes to ensure ARM64 remains eligible

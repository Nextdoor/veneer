/*
Copyright 2025 Veneer Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package preference provides parsing and generation of NodeOverlay resources
// from NodePool annotation-based instance type preferences.
//
// Users can annotate NodePools with veneer.io/preference.N annotations to express
// preferences for certain instance types or characteristics. Veneer translates
// these preferences into Karpenter NodeOverlay resources with priceAdjustment
// to influence provisioning decisions.
package preference

// Annotation prefix for all preference annotations on NodePools.
// Preferences are numbered (1-N) and parsed in order. Lower numbers have lower
// weight/priority, higher numbers override when multiple preferences match.
const AnnotationPrefix = "veneer.io/preference."

// Label constants used on preference-based NodeOverlays.
const (
	// LabelManagedBy identifies that Veneer manages this NodeOverlay.
	LabelManagedBy = "app.kubernetes.io/managed-by"

	// LabelManagedByValue is the value for the managed-by label.
	LabelManagedByValue = "veneer"

	// LabelPreferenceType identifies this overlay as a preference-based overlay.
	// This distinguishes preference overlays from cost-optimization overlays.
	LabelPreferenceType = "veneer.io/type"

	// LabelPreferenceTypeValue is the value for preference-type overlays.
	LabelPreferenceTypeValue = "preference"

	// LabelSourceNodePool identifies which NodePool this overlay was generated from.
	LabelSourceNodePool = "veneer.io/source-nodepool"

	// LabelPreferenceNumber identifies the preference number (1-N) from the annotation.
	LabelPreferenceNumber = "veneer.io/preference-number"

	// LabelDisabledKey is the label key used to create an impossible requirement
	// when disabled mode is enabled.
	LabelDisabledKey = "veneer.io/disabled"

	// LabelDisabledValue is the value used for the disabled requirement.
	LabelDisabledValue = "true"
)

// Well-known Kubernetes and Karpenter label keys that can be used in preferences.
// These are the labels that Karpenter understands for instance selection.
const (
	// LabelNodePool is the Karpenter label identifying the NodePool.
	// This is automatically added to preference overlays to scope them.
	LabelNodePool = "karpenter.sh/nodepool"

	// LabelInstanceFamily is the Karpenter label for EC2 instance family.
	LabelInstanceFamily = "karpenter.k8s.aws/instance-family"

	// LabelInstanceCategory is the Karpenter label for instance category.
	// Examples: "c" (compute), "m" (memory), "r" (memory-optimized), "g" (GPU)
	LabelInstanceCategory = "karpenter.k8s.aws/instance-category"

	// LabelInstanceGeneration is the Karpenter label for instance generation.
	// Examples: "5", "6", "7"
	LabelInstanceGeneration = "karpenter.k8s.aws/instance-generation"

	// LabelInstanceSize is the Karpenter label for instance size.
	// Examples: "large", "xlarge", "2xlarge"
	LabelInstanceSize = "karpenter.k8s.aws/instance-size"

	// LabelInstanceCPU is the Karpenter label for vCPU count.
	LabelInstanceCPU = "karpenter.k8s.aws/instance-cpu"

	// LabelInstanceCPUManufacturer is the Karpenter label for CPU manufacturer.
	// Examples: "intel", "amd", "aws" (Graviton)
	LabelInstanceCPUManufacturer = "karpenter.k8s.aws/instance-cpu-manufacturer"

	// LabelInstanceMemory is the Karpenter label for memory in MiB.
	LabelInstanceMemory = "karpenter.k8s.aws/instance-memory"

	// LabelArch is the standard Kubernetes label for CPU architecture.
	// Examples: "amd64", "arm64"
	LabelArch = "kubernetes.io/arch"

	// LabelCapacityType is the Karpenter label for capacity type.
	// Examples: "on-demand", "spot"
	LabelCapacityType = "karpenter.sh/capacity-type"

	// LabelInstanceType is the standard Kubernetes label for instance type.
	// Examples: "m5.xlarge", "c7g.large"
	LabelInstanceType = "node.kubernetes.io/instance-type"
)

// SupportedLabels is the set of labels that can be used in preference matchers.
// Using labels outside this set will result in a parse error to prevent typos
// and unsupported label usage.
var SupportedLabels = map[string]bool{
	LabelInstanceFamily:          true,
	LabelInstanceCategory:        true,
	LabelInstanceGeneration:      true,
	LabelInstanceSize:            true,
	LabelInstanceCPU:             true,
	LabelInstanceCPUManufacturer: true,
	LabelInstanceMemory:          true,
	LabelArch:                    true,
	LabelCapacityType:            true,
	LabelInstanceType:            true,
}

// Operator represents the comparison operator in a label matcher.
// This maps to Kubernetes NodeSelectorOperator values.
type Operator string

const (
	// OperatorIn matches when the label value is in the specified set.
	// Example: "karpenter.k8s.aws/instance-family=c7a,c7g"
	OperatorIn Operator = "In"

	// OperatorNotIn matches when the label value is NOT in the specified set.
	// Example: "karpenter.k8s.aws/instance-family!=m5"
	OperatorNotIn Operator = "NotIn"

	// OperatorGt matches when the label value is greater than the specified value.
	// Used for numeric labels like instance-cpu or instance-memory.
	// Example: "karpenter.k8s.aws/instance-cpu>4"
	OperatorGt Operator = "Gt"

	// OperatorLt matches when the label value is less than the specified value.
	// Used for numeric labels like instance-cpu or instance-memory.
	// Example: "karpenter.k8s.aws/instance-memory<16384"
	OperatorLt Operator = "Lt"
)

// LabelMatcher represents a single label matching condition.
// Multiple matchers in a preference are ANDed together.
type LabelMatcher struct {
	// Key is the label key to match against.
	// Must be one of the SupportedLabels.
	Key string

	// Operator is the comparison operator.
	Operator Operator

	// Values are the values to compare against.
	// For In/NotIn: multiple values allowed (any match)
	// For Gt/Lt: exactly one numeric value
	Values []string
}

// Preference represents a parsed instance type preference from a NodePool annotation.
// A preference specifies which instance types should have their effective price adjusted,
// making them more or less attractive to Karpenter's provisioning logic.
type Preference struct {
	// Number is the preference number from the annotation key (1-N).
	// This determines the weight of the generated NodeOverlay.
	// Higher numbers = higher weight = higher priority.
	Number int

	// Matchers define which instances this preference applies to.
	// Multiple matchers are ANDed together.
	Matchers []LabelMatcher

	// Adjustment is the price adjustment percentage.
	// Negative values (e.g., -20) make instances cheaper (more preferred).
	// Positive values (e.g., +40) make instances more expensive (less preferred).
	Adjustment float64

	// NodePoolName is the name of the NodePool this preference came from.
	// Used to scope the generated overlay to only affect this NodePool's instances.
	NodePoolName string
}

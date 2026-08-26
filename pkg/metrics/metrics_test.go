// Copyright 2025 Nextdoor, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import "testing"

func TestSanitizeReason(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   DecisionReason
	}{
		{
			name:   "above threshold",
			reason: "utilization 95.0% at/above threshold 95.0%",
			want:   ReasonUtilizationAboveThreshold,
		},
		{
			name:   "below threshold",
			reason: "utilization 80.0% below threshold 95.0%, capacity available",
			want:   ReasonCapacityAvailable,
		},
		{name: "no capacity", reason: "no remaining capacity (0.00 $/hour)", want: ReasonNoCapacity},
		{name: "compute disabled", reason: "compute savings plan overlays are disabled", want: ReasonDisabled},
		{name: "ec2 disabled", reason: "ec2 instance savings plan overlays are disabled", want: ReasonDisabled},
		{name: "ri disabled", reason: "reserved instance overlays are disabled", want: ReasonDisabled},
		{
			name:   "below capacity floor",
			reason: "remaining capacity 25.00 $/hour below minimum 50.00 $/hour",
			want:   ReasonBelowCapacityFloor,
		},
		{
			name:   "waiting for dwell",
			reason: "eligible for 10m0s; waiting for 15m0s minimum duration",
			want:   ReasonWaitingForDwell,
		},
		{name: "no reserved instances", reason: "no reserved instances available", want: ReasonRINotFound},
		{
			name:   "reserved instances available",
			reason: "3 reserved instances available",
			want:   ReasonRIAvailable,
		},
		{name: "unknown", reason: "unrecognized state", want: ReasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeReason(tt.reason); got != tt.want {
				t.Fatalf("SanitizeReason(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}

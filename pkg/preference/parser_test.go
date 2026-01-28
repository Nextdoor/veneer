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

package preference

import (
	"testing"
)

func TestParseNodePoolPreferences(t *testing.T) {
	tests := []struct {
		name         string
		annotations  map[string]string
		nodePoolName string
		wantPrefs    int
		wantErrors   int
		checkPrefs   func(t *testing.T, prefs []Preference)
		checkErrors  func(t *testing.T, errs []error)
	}{
		{
			name: "single preference with one matcher",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a,c7g adjust=-20%",
			},
			nodePoolName: "my-workload",
			wantPrefs:    1,
			wantErrors:   0,
			checkPrefs: func(t *testing.T, prefs []Preference) {
				p := prefs[0]
				if p.Number != 1 {
					t.Errorf("expected Number=1, got %d", p.Number)
				}
				if p.NodePoolName != "my-workload" {
					t.Errorf("expected NodePoolName=my-workload, got %s", p.NodePoolName)
				}
				if p.Adjustment != -20 {
					t.Errorf("expected Adjustment=-20, got %f", p.Adjustment)
				}
				if len(p.Matchers) != 1 {
					t.Errorf("expected 1 matcher, got %d", len(p.Matchers))
				}
				m := p.Matchers[0]
				if m.Key != LabelInstanceFamily {
					t.Errorf("expected Key=%s, got %s", LabelInstanceFamily, m.Key)
				}
				if m.Operator != OperatorIn {
					t.Errorf("expected Operator=In, got %s", m.Operator)
				}
				if len(m.Values) != 2 || m.Values[0] != "c7a" || m.Values[1] != "c7g" {
					t.Errorf("expected Values=[c7a, c7g], got %v", m.Values)
				}
			},
		},
		{
			name: "preference with architecture label",
			annotations: map[string]string{
				"veneer.io/preference.2": "kubernetes.io/arch=arm64 adjust=+40%",
			},
			nodePoolName: "test-pool",
			wantPrefs:    1,
			wantErrors:   0,
			checkPrefs: func(t *testing.T, prefs []Preference) {
				p := prefs[0]
				if p.Number != 2 {
					t.Errorf("expected Number=2, got %d", p.Number)
				}
				if p.Adjustment != 40 {
					t.Errorf("expected Adjustment=40, got %f", p.Adjustment)
				}
				m := p.Matchers[0]
				if m.Key != LabelArch {
					t.Errorf("expected Key=%s, got %s", LabelArch, m.Key)
				}
			},
		},
		{
			name: "multiple matchers in one preference",
			annotations: map[string]string{
				"veneer.io/preference.3": "karpenter.k8s.aws/instance-family=m7g kubernetes.io/arch=arm64 adjust=-30%",
			},
			nodePoolName: "multi-matcher",
			wantPrefs:    1,
			wantErrors:   0,
			checkPrefs: func(t *testing.T, prefs []Preference) {
				p := prefs[0]
				if len(p.Matchers) != 2 {
					t.Errorf("expected 2 matchers, got %d", len(p.Matchers))
				}
				if p.Adjustment != -30 {
					t.Errorf("expected Adjustment=-30, got %f", p.Adjustment)
				}
			},
		},
		{
			name: "multiple preferences sorted by number",
			annotations: map[string]string{
				"veneer.io/preference.5":  "karpenter.k8s.aws/instance-family=c7a adjust=-10%",
				"veneer.io/preference.2":  "kubernetes.io/arch=arm64 adjust=+20%",
				"veneer.io/preference.10": "karpenter.sh/capacity-type=spot adjust=-50%",
			},
			nodePoolName: "sorted",
			wantPrefs:    3,
			wantErrors:   0,
			checkPrefs: func(t *testing.T, prefs []Preference) {
				// Should be sorted by number: 2, 5, 10
				if prefs[0].Number != 2 {
					t.Errorf("expected first pref Number=2, got %d", prefs[0].Number)
				}
				if prefs[1].Number != 5 {
					t.Errorf("expected second pref Number=5, got %d", prefs[1].Number)
				}
				if prefs[2].Number != 10 {
					t.Errorf("expected third pref Number=10, got %d", prefs[2].Number)
				}
			},
		},
		{
			name: "NotIn operator",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family!=m5,m6i adjust=+10%",
			},
			nodePoolName: "notin",
			wantPrefs:    1,
			wantErrors:   0,
			checkPrefs: func(t *testing.T, prefs []Preference) {
				m := prefs[0].Matchers[0]
				if m.Operator != OperatorNotIn {
					t.Errorf("expected Operator=NotIn, got %s", m.Operator)
				}
				if len(m.Values) != 2 {
					t.Errorf("expected 2 values, got %d", len(m.Values))
				}
			},
		},
		{
			name: "Gt operator for CPU count",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-cpu>4 adjust=-15%",
			},
			nodePoolName: "gt-cpu",
			wantPrefs:    1,
			wantErrors:   0,
			checkPrefs: func(t *testing.T, prefs []Preference) {
				m := prefs[0].Matchers[0]
				if m.Operator != OperatorGt {
					t.Errorf("expected Operator=Gt, got %s", m.Operator)
				}
				if m.Key != LabelInstanceCPU {
					t.Errorf("expected Key=%s, got %s", LabelInstanceCPU, m.Key)
				}
				if len(m.Values) != 1 || m.Values[0] != "4" {
					t.Errorf("expected Values=[4], got %v", m.Values)
				}
			},
		},
		{
			name: "Lt operator for memory",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-memory<16384 adjust=+5%",
			},
			nodePoolName: "lt-mem",
			wantPrefs:    1,
			wantErrors:   0,
			checkPrefs: func(t *testing.T, prefs []Preference) {
				m := prefs[0].Matchers[0]
				if m.Operator != OperatorLt {
					t.Errorf("expected Operator=Lt, got %s", m.Operator)
				}
				if m.Key != LabelInstanceMemory {
					t.Errorf("expected Key=%s, got %s", LabelInstanceMemory, m.Key)
				}
			},
		},
		{
			name: "decimal adjustment",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-12.5%",
			},
			nodePoolName: "decimal",
			wantPrefs:    1,
			wantErrors:   0,
			checkPrefs: func(t *testing.T, prefs []Preference) {
				if prefs[0].Adjustment != -12.5 {
					t.Errorf("expected Adjustment=-12.5, got %f", prefs[0].Adjustment)
				}
			},
		},
		{
			name: "ignores non-preference annotations",
			annotations: map[string]string{
				"veneer.io/preference.1":  "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
				"veneer.io/other":         "something",
				"kubernetes.io/something": "else",
			},
			nodePoolName: "mixed",
			wantPrefs:    1,
			wantErrors:   0,
		},
		{
			name:         "no preferences",
			annotations:  map[string]string{},
			nodePoolName: "empty",
			wantPrefs:    0,
			wantErrors:   0,
		},
		{
			name: "invalid preference number - not a number",
			annotations: map[string]string{
				"veneer.io/preference.abc": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
			},
			nodePoolName: "invalid-num",
			wantPrefs:    0,
			wantErrors:   1,
		},
		{
			name: "invalid preference number - zero",
			annotations: map[string]string{
				"veneer.io/preference.0": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
			},
			nodePoolName: "zero-num",
			wantPrefs:    0,
			wantErrors:   1,
		},
		{
			name: "invalid preference number - negative",
			annotations: map[string]string{
				"veneer.io/preference.-1": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
			},
			nodePoolName: "neg-num",
			wantPrefs:    0,
			wantErrors:   1,
		},
		{
			name: "missing adjustment",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a",
			},
			nodePoolName: "missing-adj",
			wantPrefs:    0,
			wantErrors:   1,
		},
		{
			name: "invalid adjustment format",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=20",
			},
			nodePoolName: "bad-adj",
			wantPrefs:    0,
			wantErrors:   1,
		},
		{
			name: "unsupported label key",
			annotations: map[string]string{
				"veneer.io/preference.1": "custom.label/foo=bar adjust=-10%",
			},
			nodePoolName: "unsupported",
			wantPrefs:    0,
			wantErrors:   1,
		},
		{
			name: "empty value",
			annotations: map[string]string{
				"veneer.io/preference.1": "",
			},
			nodePoolName: "empty-val",
			wantPrefs:    0,
			wantErrors:   1,
		},
		{
			name: "missing matchers",
			annotations: map[string]string{
				"veneer.io/preference.1": "adjust=-20%",
			},
			nodePoolName: "no-matchers",
			wantPrefs:    0,
			wantErrors:   1,
		},
		{
			name: "partial success - one valid, one invalid",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
				"veneer.io/preference.2": "invalid",
			},
			nodePoolName: "partial",
			wantPrefs:    1,
			wantErrors:   1,
			checkPrefs: func(t *testing.T, prefs []Preference) {
				if prefs[0].Number != 1 {
					t.Errorf("expected valid pref Number=1, got %d", prefs[0].Number)
				}
			},
		},
		{
			name: "instance type label",
			annotations: map[string]string{
				"veneer.io/preference.1": "node.kubernetes.io/instance-type=m5.xlarge,m5.2xlarge adjust=-25%",
			},
			nodePoolName: "instance-type",
			wantPrefs:    1,
			wantErrors:   0,
			checkPrefs: func(t *testing.T, prefs []Preference) {
				m := prefs[0].Matchers[0]
				if m.Key != LabelInstanceType {
					t.Errorf("expected Key=%s, got %s", LabelInstanceType, m.Key)
				}
			},
		},
		{
			name: "CPU manufacturer label",
			annotations: map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-cpu-manufacturer=aws adjust=-30%",
			},
			nodePoolName: "cpu-mfr",
			wantPrefs:    1,
			wantErrors:   0,
			checkPrefs: func(t *testing.T, prefs []Preference) {
				m := prefs[0].Matchers[0]
				if m.Key != LabelInstanceCPUManufacturer {
					t.Errorf("expected Key=%s, got %s", LabelInstanceCPUManufacturer, m.Key)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefs, errs := ParseNodePoolPreferences(tt.annotations, tt.nodePoolName)

			if len(prefs) != tt.wantPrefs {
				t.Errorf("expected %d preferences, got %d", tt.wantPrefs, len(prefs))
			}
			if len(errs) != tt.wantErrors {
				t.Errorf("expected %d errors, got %d: %v", tt.wantErrors, len(errs), errs)
			}

			if tt.checkPrefs != nil && len(prefs) > 0 {
				tt.checkPrefs(t, prefs)
			}
			if tt.checkErrors != nil && len(errs) > 0 {
				tt.checkErrors(t, errs)
			}
		})
	}
}

func TestParsePreferenceValue(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantErr   bool
		checkPref func(t *testing.T, p *Preference)
	}{
		{
			name:    "valid single matcher",
			value:   "karpenter.k8s.aws/instance-family=c7a adjust=-20%",
			wantErr: false,
			checkPref: func(t *testing.T, p *Preference) {
				if len(p.Matchers) != 1 {
					t.Errorf("expected 1 matcher, got %d", len(p.Matchers))
				}
				if p.Adjustment != -20 {
					t.Errorf("expected Adjustment=-20, got %f", p.Adjustment)
				}
			},
		},
		{
			name:    "extra whitespace",
			value:   "  karpenter.k8s.aws/instance-family=c7a   adjust=-20%  ",
			wantErr: false,
			checkPref: func(t *testing.T, p *Preference) {
				if len(p.Matchers) != 1 {
					t.Errorf("expected 1 matcher, got %d", len(p.Matchers))
				}
			},
		},
		{
			name:    "empty string",
			value:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			value:   "   ",
			wantErr: true,
		},
		{
			name:    "adjustment without matcher",
			value:   "adjust=-20%",
			wantErr: true,
		},
		{
			name:    "matcher without adjustment",
			value:   "karpenter.k8s.aws/instance-family=c7a",
			wantErr: true,
		},
		{
			name:    "invalid adjustment - no percent",
			value:   "karpenter.k8s.aws/instance-family=c7a adjust=-20",
			wantErr: true,
		},
		{
			name:    "invalid adjustment - no value",
			value:   "karpenter.k8s.aws/instance-family=c7a adjust=%",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pref, err := parsePreferenceValue(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("expected error=%v, got %v", tt.wantErr, err)
			}
			if tt.checkPref != nil && pref != nil {
				tt.checkPref(t, pref)
			}
		})
	}
}

func TestParseMatcher(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
		check   func(t *testing.T, m *LabelMatcher)
	}{
		{
			name:    "In operator single value",
			expr:    "karpenter.k8s.aws/instance-family=c7a",
			wantErr: false,
			check: func(t *testing.T, m *LabelMatcher) {
				if m.Operator != OperatorIn {
					t.Errorf("expected In, got %s", m.Operator)
				}
				if len(m.Values) != 1 || m.Values[0] != "c7a" {
					t.Errorf("expected [c7a], got %v", m.Values)
				}
			},
		},
		{
			name:    "In operator multiple values",
			expr:    "karpenter.k8s.aws/instance-family=c7a,c7g,m7g",
			wantErr: false,
			check: func(t *testing.T, m *LabelMatcher) {
				if len(m.Values) != 3 {
					t.Errorf("expected 3 values, got %d", len(m.Values))
				}
			},
		},
		{
			name:    "NotIn operator",
			expr:    "karpenter.k8s.aws/instance-family!=m5",
			wantErr: false,
			check: func(t *testing.T, m *LabelMatcher) {
				if m.Operator != OperatorNotIn {
					t.Errorf("expected NotIn, got %s", m.Operator)
				}
			},
		},
		{
			name:    "Gt operator",
			expr:    "karpenter.k8s.aws/instance-cpu>8",
			wantErr: false,
			check: func(t *testing.T, m *LabelMatcher) {
				if m.Operator != OperatorGt {
					t.Errorf("expected Gt, got %s", m.Operator)
				}
				if m.Values[0] != "8" {
					t.Errorf("expected value 8, got %s", m.Values[0])
				}
			},
		},
		{
			name:    "Lt operator",
			expr:    "karpenter.k8s.aws/instance-memory<32768",
			wantErr: false,
			check: func(t *testing.T, m *LabelMatcher) {
				if m.Operator != OperatorLt {
					t.Errorf("expected Lt, got %s", m.Operator)
				}
			},
		},
		{
			name:    "unsupported label",
			expr:    "foo.bar/baz=qux",
			wantErr: true,
		},
		{
			name:    "empty value",
			expr:    "karpenter.k8s.aws/instance-family=",
			wantErr: true,
		},
		{
			name:    "no operator",
			expr:    "karpenter.k8s.aws/instance-family",
			wantErr: true,
		},
		{
			name:    "Gt with non-numeric value",
			expr:    "karpenter.k8s.aws/instance-cpu>large",
			wantErr: true,
		},
		{
			name:    ">= not supported",
			expr:    "karpenter.k8s.aws/instance-cpu>=8",
			wantErr: true,
		},
		{
			name:    "<= not supported",
			expr:    "karpenter.k8s.aws/instance-cpu<=8",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := parseMatcher(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("expected error=%v, got %v", tt.wantErr, err)
			}
			if tt.check != nil && m != nil {
				tt.check(t, m)
			}
		})
	}
}

func TestParseAdjustment(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		want    float64
		wantErr bool
	}{
		{
			name:    "negative integer",
			expr:    "adjust=-20%",
			want:    -20,
			wantErr: false,
		},
		{
			name:    "positive with plus sign",
			expr:    "adjust=+40%",
			want:    40,
			wantErr: false,
		},
		{
			name:    "positive without sign",
			expr:    "adjust=15%",
			want:    15,
			wantErr: false,
		},
		{
			name:    "zero",
			expr:    "adjust=0%",
			want:    0,
			wantErr: false,
		},
		{
			name:    "decimal negative",
			expr:    "adjust=-12.5%",
			want:    -12.5,
			wantErr: false,
		},
		{
			name:    "decimal positive",
			expr:    "adjust=+7.25%",
			want:    7.25,
			wantErr: false,
		},
		{
			name:    "missing percent",
			expr:    "adjust=-20",
			wantErr: true,
		},
		{
			name:    "missing value",
			expr:    "adjust=%",
			wantErr: true,
		},
		{
			name:    "invalid format",
			expr:    "adjustment=-20%",
			wantErr: true,
		},
		{
			name:    "non-numeric",
			expr:    "adjust=abc%",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAdjustment(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("expected error=%v, got %v", tt.wantErr, err)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("expected %f, got %f", tt.want, got)
			}
		})
	}
}

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

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		wantErr    bool
		validate   func(*testing.T, *Config)
	}{
		{
			name: "valid config with all fields",
			configYAML: `
prometheusUrl: "http://prometheus:9090"
logLevel: "debug"
metricsBindAddress: ":8080"
healthProbeBindAddress: ":8081"
aws:
  accountId: "123456789012"
  region: "us-west-2"
`,
			wantErr: false,
			validate: func(t *testing.T, c *Config) {
				if c.PrometheusURL != "http://prometheus:9090" {
					t.Errorf("PrometheusURL = %q, want %q", c.PrometheusURL, "http://prometheus:9090")
				}
				if c.LogLevel != "debug" {
					t.Errorf("LogLevel = %q, want %q", c.LogLevel, "debug")
				}
				if c.AWS.AccountID != "123456789012" {
					t.Errorf("AWS.AccountID = %q, want %q", c.AWS.AccountID, "123456789012")
				}
				if c.AWS.Region != "us-west-2" {
					t.Errorf("AWS.Region = %q, want %q", c.AWS.Region, "us-west-2")
				}
			},
		},
		{
			name: "minimal valid config",
			configYAML: `
prometheusUrl: "http://prom:9090"
aws:
  accountId: "123456789012"
  region: "us-east-1"
`,
			wantErr: false,
			validate: func(t *testing.T, c *Config) {
				if c.PrometheusURL != "http://prom:9090" {
					t.Errorf("PrometheusURL = %q, want %q", c.PrometheusURL, "http://prom:9090")
				}
				// Check defaults
				if c.LogLevel != DefaultLogLevel {
					t.Errorf("LogLevel = %q, want default %q", c.LogLevel, DefaultLogLevel)
				}
			},
		},
		{
			name: "invalid log level",
			configYAML: `
prometheusUrl: "http://prom:9090"
logLevel: "invalid"
aws:
  accountId: "123456789012"
  region: "us-west-2"
`,
			wantErr: true,
		},
		{
			name: "missing AWS account ID",
			configYAML: `
prometheusUrl: "http://prom:9090"
aws:
  region: "us-west-2"
`,
			wantErr: true,
		},
		{
			name: "missing AWS region",
			configYAML: `
prometheusUrl: "http://prom:9090"
aws:
  accountId: "123456789012"
`,
			wantErr: true,
		},
		{
			name: "invalid AWS account ID - not 12 digits",
			configYAML: `
prometheusUrl: "http://prom:9090"
aws:
  accountId: "12345"
  region: "us-west-2"
`,
			wantErr: true,
		},
		{
			name: "invalid AWS account ID - contains letters",
			configYAML: `
prometheusUrl: "http://prom:9090"
aws:
  accountId: "12345678901a"
  region: "us-west-2"
`,
			wantErr: true,
		},
		{
			name: "missing prometheus URL uses default",
			configYAML: `
logLevel: "info"
aws:
  accountId: "123456789012"
  region: "us-west-2"
`,
			wantErr: false,
			validate: func(t *testing.T, c *Config) {
				if c.PrometheusURL != DefaultPrometheusURL {
					t.Errorf("PrometheusURL = %q, want default %q", c.PrometheusURL, DefaultPrometheusURL)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			if err := os.WriteFile(configPath, []byte(tt.configYAML), 0644); err != nil {
				t.Fatalf("failed to write temp config: %v", err)
			}

			// Load config
			cfg, err := Load(configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				PrometheusURL: "http://prometheus:9090",
				LogLevel:      "info",
				AWS: AWSConfig{
					AccountID: "123456789012",
					Region:    "us-west-2",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid log level",
			config: Config{
				PrometheusURL: "http://prometheus:9090",
				LogLevel:      "trace",
				AWS: AWSConfig{
					AccountID: "123456789012",
					Region:    "us-west-2",
				},
			},
			wantErr: true,
		},
		{
			name: "all valid log levels",
			config: Config{
				PrometheusURL: "http://prometheus:9090",
				LogLevel:      "debug",
				AWS: AWSConfig{
					AccountID: "123456789012",
					Region:    "us-west-2",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load() expected error for nonexistent file, got nil")
	}
}

func TestValidateEmptyPrometheusURL(t *testing.T) {
	config := Config{
		PrometheusURL: "",
		LogLevel:      "info",
		AWS: AWSConfig{
			AccountID: "123456789012",
			Region:    "us-west-2",
		},
	}
	err := config.Validate()
	if err == nil {
		t.Error("Validate() expected error for empty PrometheusURL, got nil")
	}
}

func TestEnvironmentVariableOverrides(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configYAML := `
prometheusUrl: "http://default:9090"
aws:
  accountId: "111111111111"
  region: "us-east-1"
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	// Set environment variables
	_ = os.Setenv("VENEER_PROMETHEUS_URL", "http://override:9090")
	_ = os.Setenv("VENEER_AWS_ACCOUNT_ID", "222222222222")
	_ = os.Setenv("VENEER_AWS_REGION", "us-west-2")
	defer func() {
		_ = os.Unsetenv("VENEER_PROMETHEUS_URL")
		_ = os.Unsetenv("VENEER_AWS_ACCOUNT_ID")
		_ = os.Unsetenv("VENEER_AWS_REGION")
	}()

	// Load config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Environment variables should override file values
	if cfg.PrometheusURL != "http://override:9090" {
		t.Errorf("PrometheusURL = %q, want %q (env var override)", cfg.PrometheusURL, "http://override:9090")
	}
	if cfg.AWS.AccountID != "222222222222" {
		t.Errorf("AWS.AccountID = %q, want %q (env var override)", cfg.AWS.AccountID, "222222222222")
	}
	if cfg.AWS.Region != "us-west-2" {
		t.Errorf("AWS.Region = %q, want %q (env var override)", cfg.AWS.Region, "us-west-2")
	}
}

func TestOverlayManagementDefaults(t *testing.T) {
	// Create minimal config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configYAML := `
prometheusUrl: "http://prometheus:9090"
aws:
  accountId: "123456789012"
  region: "us-west-2"
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify defaults are applied
	if cfg.Overlays.UtilizationThreshold != DefaultOverlayUtilizationThreshold {
		t.Errorf(
			"UtilizationThreshold = %f, want %f",
			cfg.Overlays.UtilizationThreshold,
			DefaultOverlayUtilizationThreshold,
		)
	}
	if cfg.Overlays.Weights.ReservedInstance != DefaultOverlayWeightReservedInstance {
		t.Errorf(
			"ReservedInstance weight = %d, want %d",
			cfg.Overlays.Weights.ReservedInstance,
			DefaultOverlayWeightReservedInstance,
		)
	}
	if cfg.Overlays.Weights.EC2InstanceSavingsPlan != DefaultOverlayWeightEC2InstanceSavingsPlan {
		t.Errorf(
			"EC2InstanceSavingsPlan weight = %d, want %d",
			cfg.Overlays.Weights.EC2InstanceSavingsPlan,
			DefaultOverlayWeightEC2InstanceSavingsPlan,
		)
	}
	if cfg.Overlays.Weights.ComputeSavingsPlan != DefaultOverlayWeightComputeSavingsPlan {
		t.Errorf(
			"ComputeSavingsPlan weight = %d, want %d",
			cfg.Overlays.Weights.ComputeSavingsPlan,
			DefaultOverlayWeightComputeSavingsPlan,
		)
	}
}

func TestOverlayManagementCustomValues(t *testing.T) {
	// Create config with custom overlay values
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configYAML := `
prometheusUrl: "http://prometheus:9090"
aws:
  accountId: "123456789012"
  region: "us-west-2"
overlays:
  utilizationThreshold: 90.0
  weights:
    reservedInstance: 100
    ec2InstanceSavingsPlan: 50
    computeSavingsPlan: 25
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify custom values are loaded
	if cfg.Overlays.UtilizationThreshold != 90.0 {
		t.Errorf("UtilizationThreshold = %f, want 90.0", cfg.Overlays.UtilizationThreshold)
	}
	if cfg.Overlays.Weights.ReservedInstance != 100 {
		t.Errorf("ReservedInstance weight = %d, want 100", cfg.Overlays.Weights.ReservedInstance)
	}
	if cfg.Overlays.Weights.EC2InstanceSavingsPlan != 50 {
		t.Errorf("EC2InstanceSavingsPlan weight = %d, want 50", cfg.Overlays.Weights.EC2InstanceSavingsPlan)
	}
	if cfg.Overlays.Weights.ComputeSavingsPlan != 25 {
		t.Errorf("ComputeSavingsPlan weight = %d, want 25", cfg.Overlays.Weights.ComputeSavingsPlan)
	}
}

func TestValidateOverlayManagement(t *testing.T) {
	tests := []struct {
		name    string
		config  OverlayManagementConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: OverlayManagementConfig{
				UtilizationThreshold: 95.0,
				Weights: OverlayWeightsConfig{
					ReservedInstance:       30,
					EC2InstanceSavingsPlan: 20,
					ComputeSavingsPlan:     10,
				},
			},
			wantErr: false,
		},
		{
			name: "threshold too high",
			config: OverlayManagementConfig{
				UtilizationThreshold: 150.0,
			},
			wantErr: true,
		},
		{
			name: "threshold negative",
			config: OverlayManagementConfig{
				UtilizationThreshold: -10.0,
			},
			wantErr: true,
		},
		{
			name: "negative RI weight",
			config: OverlayManagementConfig{
				UtilizationThreshold: 95.0,
				Weights: OverlayWeightsConfig{
					ReservedInstance: -1,
				},
			},
			wantErr: true,
		},
		{
			name: "zero weights are valid",
			config: OverlayManagementConfig{
				UtilizationThreshold: 95.0,
				Weights: OverlayWeightsConfig{
					ReservedInstance:       0,
					EC2InstanceSavingsPlan: 0,
					ComputeSavingsPlan:     0,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				PrometheusURL: "http://prometheus:9090",
				AWS: AWSConfig{
					AccountID: "123456789012",
					Region:    "us-west-2",
				},
				Overlays: tt.config,
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

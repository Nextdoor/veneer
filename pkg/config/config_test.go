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

package config

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
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
				Overlays: validOverlayManagementConfig(),
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
				Overlays: validOverlayManagementConfig(),
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
	if !cfg.Overlays.ReservedInstance.Enabled ||
		cfg.Overlays.ReservedInstance.PriceAdjustment != DefaultOverlayReservedInstancePriceAdjustment {
		t.Errorf("ReservedInstance config = %+v, want enabled with %q adjustment",
			cfg.Overlays.ReservedInstance, DefaultOverlayReservedInstancePriceAdjustment)
	}
	if !cfg.Overlays.EC2InstanceSavingsPlan.Enabled ||
		cfg.Overlays.EC2InstanceSavingsPlan.PriceAdjustment != DefaultOverlayEC2InstanceSavingsPlanPriceAdjustment {
		t.Errorf("EC2InstanceSavingsPlan config = %+v, want enabled with %q adjustment",
			cfg.Overlays.EC2InstanceSavingsPlan, DefaultOverlayEC2InstanceSavingsPlanPriceAdjustment)
	}
	if !cfg.Overlays.ComputeSavingsPlan.Enabled ||
		cfg.Overlays.ComputeSavingsPlan.PriceAdjustment != DefaultOverlayComputeSavingsPlanPriceAdjustment {
		t.Errorf("ComputeSavingsPlan config = %+v, want enabled with %q adjustment",
			cfg.Overlays.ComputeSavingsPlan, DefaultOverlayComputeSavingsPlanPriceAdjustment)
	}
	if cfg.Overlays.ComputeSavingsPlan.MinRemainingCapacityDollars != DefaultOverlayComputeSPMinRemainingCapacityDollars {
		t.Errorf("ComputeSavingsPlan floor = %f, want %f",
			cfg.Overlays.ComputeSavingsPlan.MinRemainingCapacityDollars,
			DefaultOverlayComputeSPMinRemainingCapacityDollars)
	}
	if cfg.Overlays.ComputeSavingsPlan.MinBelowThresholdDuration != DefaultOverlayComputeSPMinBelowThresholdDuration {
		t.Errorf("ComputeSavingsPlan duration = %s, want %s",
			cfg.Overlays.ComputeSavingsPlan.MinBelowThresholdDuration,
			DefaultOverlayComputeSPMinBelowThresholdDuration)
	}
	if len(cfg.Overlays.ComputeSavingsPlan.NodePoolSelector.Names) != 0 {
		t.Errorf("ComputeSavingsPlan NodePool names = %v, want empty", cfg.Overlays.ComputeSavingsPlan.NodePoolSelector.Names)
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
  reservedInstance:
    enabled: false
    priceAdjustment: "-60%"
  ec2InstanceSavingsPlan:
    enabled: false
    priceAdjustment: "-40%"
  computeSavingsPlan:
    enabled: false
    priceAdjustment: "-25%"
    nodePoolSelector:
      names: ["on-demand", "batch"]
    minRemainingCapacityDollars: 75.5
    minBelowThresholdDuration: 30m
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
	if cfg.Overlays.ReservedInstance.Enabled || cfg.Overlays.ReservedInstance.PriceAdjustment != "-60%" {
		t.Errorf("ReservedInstance config = %+v, want disabled with -60%% adjustment", cfg.Overlays.ReservedInstance)
	}
	if cfg.Overlays.EC2InstanceSavingsPlan.Enabled ||
		cfg.Overlays.EC2InstanceSavingsPlan.PriceAdjustment != "-40%" {
		t.Errorf(
			"EC2InstanceSavingsPlan config = %+v, want disabled with -40%% adjustment",
			cfg.Overlays.EC2InstanceSavingsPlan,
		)
	}
	if cfg.Overlays.ComputeSavingsPlan.Enabled || cfg.Overlays.ComputeSavingsPlan.PriceAdjustment != "-25%" {
		t.Errorf("ComputeSavingsPlan config = %+v, want disabled with -25%% adjustment", cfg.Overlays.ComputeSavingsPlan)
	}
	if got := cfg.Overlays.ComputeSavingsPlan.NodePoolSelector.Names; len(got) != 2 ||
		got[0] != "on-demand" || got[1] != "batch" {
		t.Errorf("ComputeSavingsPlan NodePool names = %v, want [on-demand batch]", got)
	}
	if cfg.Overlays.ComputeSavingsPlan.MinRemainingCapacityDollars != 75.5 {
		t.Errorf("ComputeSavingsPlan floor = %f, want 75.5", cfg.Overlays.ComputeSavingsPlan.MinRemainingCapacityDollars)
	}
	if cfg.Overlays.ComputeSavingsPlan.MinBelowThresholdDuration != 30*time.Minute {
		t.Errorf("ComputeSavingsPlan duration = %s, want 30m", cfg.Overlays.ComputeSavingsPlan.MinBelowThresholdDuration)
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

func TestOverlayTypeEnvironmentVariableOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configYAML := `
aws:
  accountId: "123456789012"
  region: "us-west-2"
overlays:
  computeSavingsPlan:
    nodePoolSelector:
      names: ["on-demand"]
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv(EnvOverlayReservedInstanceEnabled, "false")
	t.Setenv(EnvOverlayReservedInstancePriceAdjustment, "-65%")
	t.Setenv(EnvOverlayEC2InstanceSavingsPlanEnabled, "false")
	t.Setenv(EnvOverlayEC2InstanceSavingsPlanPriceAdjustment, "-45%")
	t.Setenv(EnvOverlayComputeSavingsPlanEnabled, "false")
	t.Setenv(EnvOverlayComputeSavingsPlanPriceAdjustment, "-35%")
	t.Setenv(EnvOverlayComputeSPMinRemainingCapacityDollars, "80.5")
	t.Setenv(EnvOverlayComputeSPMinBelowThresholdDuration, "45m")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Overlays.ReservedInstance.Enabled || cfg.Overlays.ReservedInstance.PriceAdjustment != "-65%" {
		t.Errorf("ReservedInstance env override = %+v", cfg.Overlays.ReservedInstance)
	}
	if cfg.Overlays.EC2InstanceSavingsPlan.Enabled || cfg.Overlays.EC2InstanceSavingsPlan.PriceAdjustment != "-45%" {
		t.Errorf("EC2InstanceSavingsPlan env override = %+v", cfg.Overlays.EC2InstanceSavingsPlan)
	}
	if cfg.Overlays.ComputeSavingsPlan.Enabled || cfg.Overlays.ComputeSavingsPlan.PriceAdjustment != "-35%" {
		t.Errorf("ComputeSavingsPlan env override = %+v", cfg.Overlays.ComputeSavingsPlan)
	}
	if cfg.Overlays.ComputeSavingsPlan.MinRemainingCapacityDollars != 80.5 {
		t.Errorf("ComputeSavingsPlan floor = %f, want 80.5", cfg.Overlays.ComputeSavingsPlan.MinRemainingCapacityDollars)
	}
	if cfg.Overlays.ComputeSavingsPlan.MinBelowThresholdDuration != 45*time.Minute {
		t.Errorf("ComputeSavingsPlan duration = %s, want 45m", cfg.Overlays.ComputeSavingsPlan.MinBelowThresholdDuration)
	}
	if got := cfg.Overlays.ComputeSavingsPlan.NodePoolSelector.Names; len(got) != 1 || got[0] != "on-demand" {
		t.Errorf("NodePool names = %v, want file value [on-demand]; names must not have an env override", got)
	}
}

func TestLoadRejectsUnitlessComputeSPDuration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configYAML := `
prometheusUrl: "http://prometheus:9090"
aws:
  accountId: "123456789012"
  region: "us-west-2"
overlays:
  computeSavingsPlan:
    minBelowThresholdDuration: 15
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(configPath); err == nil {
		t.Fatal("expected unitless duration to be rejected")
	}
}

func TestValidateOverlayTypeConfiguration(t *testing.T) {
	baseConfig := func() Config {
		return Config{
			PrometheusURL: "http://prometheus:9090",
			LogLevel:      "info",
			AWS: AWSConfig{
				AccountID: "123456789012",
				Region:    "us-west-2",
			},
			Overlays: OverlayManagementConfig{
				ReservedInstance:       CapacityOverlayConfig{PriceAdjustment: "-50%"},
				EC2InstanceSavingsPlan: CapacityOverlayConfig{PriceAdjustment: "-50%"},
				ComputeSavingsPlan:     ComputeSavingsPlanOverlayConfig{PriceAdjustment: "-50%"},
			},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "valid boundaries", mutate: func(c *Config) {
			c.Overlays.ReservedInstance.PriceAdjustment = "-99.999%"
			c.Overlays.EC2InstanceSavingsPlan.PriceAdjustment = "-0.001%"
			c.Overlays.ComputeSavingsPlan.MinRemainingCapacityDollars = 0
			c.Overlays.ComputeSavingsPlan.MinBelowThresholdDuration = 0
		}},
		{name: "negative one hundred percent rejected", mutate: func(c *Config) {
			c.Overlays.ReservedInstance.PriceAdjustment = "-100%"
		}, wantErr: true},
		{name: "zero percent rejected", mutate: func(c *Config) {
			c.Overlays.EC2InstanceSavingsPlan.PriceAdjustment = "0%"
		}, wantErr: true},
		{name: "positive adjustment rejected", mutate: func(c *Config) {
			c.Overlays.ComputeSavingsPlan.PriceAdjustment = "+10%"
		}, wantErr: true},
		{name: "absolute price rejected", mutate: func(c *Config) {
			c.Overlays.ComputeSavingsPlan.PriceAdjustment = "0.00"
		}, wantErr: true},
		{name: "empty adjustment rejected", mutate: func(c *Config) {
			c.Overlays.ReservedInstance.PriceAdjustment = ""
		}, wantErr: true},
		{name: "nan adjustment rejected", mutate: func(c *Config) {
			c.Overlays.ComputeSavingsPlan.PriceAdjustment = "NaN%"
		}, wantErr: true},
		{name: "exponent adjustment rejected", mutate: func(c *Config) {
			c.Overlays.ComputeSavingsPlan.PriceAdjustment = "-1e1%"
		}, wantErr: true},
		{name: "negative floor rejected", mutate: func(c *Config) {
			c.Overlays.ComputeSavingsPlan.MinRemainingCapacityDollars = -1
		}, wantErr: true},
		{name: "nan floor rejected", mutate: func(c *Config) {
			c.Overlays.ComputeSavingsPlan.MinRemainingCapacityDollars = math.NaN()
		}, wantErr: true},
		{name: "invalid NodePool name rejected", mutate: func(c *Config) {
			c.Overlays.ComputeSavingsPlan.NodePoolSelector.Names = []string{"Invalid_Name"}
		}, wantErr: true},
		{name: "duplicate NodePool name rejected", mutate: func(c *Config) {
			c.Overlays.ComputeSavingsPlan.NodePoolSelector.Names = []string{"on-demand", "on-demand"}
		}, wantErr: true},
		{name: "negative duration rejected", mutate: func(c *Config) {
			c.Overlays.ComputeSavingsPlan.MinBelowThresholdDuration = -time.Second
		}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func validOverlayManagementConfig() OverlayManagementConfig {
	return OverlayManagementConfig{
		ReservedInstance: CapacityOverlayConfig{
			Enabled:         true,
			PriceAdjustment: DefaultOverlayReservedInstancePriceAdjustment,
		},
		EC2InstanceSavingsPlan: CapacityOverlayConfig{
			Enabled:         true,
			PriceAdjustment: DefaultOverlayEC2InstanceSavingsPlanPriceAdjustment,
		},
		ComputeSavingsPlan: ComputeSavingsPlanOverlayConfig{
			Enabled:                     true,
			PriceAdjustment:             DefaultOverlayComputeSavingsPlanPriceAdjustment,
			MinRemainingCapacityDollars: DefaultOverlayComputeSPMinRemainingCapacityDollars,
			MinBelowThresholdDuration:   DefaultOverlayComputeSPMinBelowThresholdDuration,
		},
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
			tt.config.ReservedInstance.PriceAdjustment = DefaultOverlayReservedInstancePriceAdjustment
			tt.config.EC2InstanceSavingsPlan.PriceAdjustment = DefaultOverlayEC2InstanceSavingsPlanPriceAdjustment
			tt.config.ComputeSavingsPlan.PriceAdjustment = DefaultOverlayComputeSavingsPlanPriceAdjustment
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

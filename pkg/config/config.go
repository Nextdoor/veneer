/*
Copyright 2025 Karve Contributors.

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

// Package config provides configuration management for the Karve controller.
//
// Configuration can be loaded from YAML files or environment variables.
// Uses Viper for robust configuration management with automatic env binding.
package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Configuration key constants for viper SetDefault and BindEnv calls.
const (
	KeyPrometheusURL                       = "prometheusUrl"
	KeyLogLevel                            = "logLevel"
	KeyMetricsBindAddress                  = "metricsBindAddress"
	KeyHealthProbeBindAddress              = "healthProbeBindAddress"
	KeyOverlayUtilizationThreshold         = "overlayManagement.utilizationThreshold"
	KeyOverlayWeightReservedInstance       = "overlayManagement.weights.reservedInstance"
	KeyOverlayWeightEC2InstanceSavingsPlan = "overlayManagement.weights.ec2InstanceSavingsPlan"
	KeyOverlayWeightComputeSavingsPlan     = "overlayManagement.weights.computeSavingsPlan"
)

// Environment variable name constants.
const (
	EnvPrometheusURL          = "KARVE_PROMETHEUS_URL"
	EnvLogLevel               = "KARVE_LOG_LEVEL"
	EnvMetricsBindAddress     = "KARVE_METRICS_BIND_ADDRESS"
	EnvHealthProbeBindAddress = "KARVE_HEALTH_PROBE_BIND_ADDRESS"
	EnvPrefix                 = "KARVE"
)

// Default configuration values.
const (
	DefaultPrometheusURL                       = "http://prometheus:9090"
	DefaultLogLevel                            = "info"
	DefaultMetricsBindAddress                  = ":8080"
	DefaultHealthProbeBindAddress              = ":8081"
	DefaultOverlayUtilizationThreshold         = 95.0 // Delete overlays at 95% utilization
	DefaultOverlayWeightReservedInstance       = 30   // Highest priority (most specific)
	DefaultOverlayWeightEC2InstanceSavingsPlan = 20   // Medium priority (family-specific)
	DefaultOverlayWeightComputeSavingsPlan     = 10   // Lowest priority (global)
)

// Config represents the complete controller configuration.
type Config struct {
	// PrometheusURL is the URL of the Prometheus server to query for Lumina metrics.
	PrometheusURL string `yaml:"prometheusUrl,omitempty"`

	// LogLevel controls the verbosity of logs.
	// Valid values: debug, info, warn, error
	LogLevel string `yaml:"logLevel,omitempty"`

	// MetricsBindAddress is the address the metrics endpoint binds to.
	MetricsBindAddress string `yaml:"metricsBindAddress,omitempty"`

	// HealthProbeBindAddress is the address the health probe endpoint binds to.
	HealthProbeBindAddress string `yaml:"healthProbeBindAddress,omitempty"`

	// OverlayManagement configures NodeOverlay lifecycle behavior.
	OverlayManagement OverlayManagementConfig `yaml:"overlayManagement,omitempty"`
}

// OverlayManagementConfig controls when overlays are created/deleted based on capacity utilization.
//
// Overlays are created when Savings Plans or Reserved Instances have available capacity,
// making Karpenter prefer on-demand instances that will receive pre-paid coverage.
// Overlays are deleted when capacity is exhausted (at utilization threshold).
type OverlayManagementConfig struct {
	// UtilizationThreshold is the SP/RI utilization percentage at which overlays are deleted.
	// When utilization reaches this threshold, overlays are removed to prevent over-provisioning
	// beyond available pre-paid capacity.
	//
	// Default: 95.0 (delete overlays at 95% utilization)
	// Valid range: 0-100
	UtilizationThreshold float64 `yaml:"utilizationThreshold,omitempty"`

	// Weights controls overlay precedence when multiple overlays target the same instances.
	// Higher weights win. Reserved Instances (most specific) should have highest weight,
	// followed by EC2 Instance SPs (family-specific), then Compute SPs (global).
	Weights OverlayWeightsConfig `yaml:"weights,omitempty"`
}

// OverlayWeightsConfig defines precedence for different capacity types.
//
// Weight determines which overlay takes effect when multiple overlays target the same instances.
// The overlay with the highest weight wins. This ensures more specific capacity (RIs) takes
// precedence over less specific capacity (global Compute SPs).
type OverlayWeightsConfig struct {
	// ReservedInstance weight for RI-backed overlays (instance-type specific).
	// Default: 30 (highest priority)
	ReservedInstance int `yaml:"reservedInstance,omitempty"`

	// EC2InstanceSavingsPlan weight for EC2 Instance SP overlays (family-specific).
	// Default: 20 (medium priority)
	EC2InstanceSavingsPlan int `yaml:"ec2InstanceSavingsPlan,omitempty"`

	// ComputeSavingsPlan weight for Compute SP overlays (global, all families).
	// Default: 10 (lowest priority)
	ComputeSavingsPlan int `yaml:"computeSavingsPlan,omitempty"`
}

// Load loads configuration from a YAML file and validates it.
//
// Configuration precedence (highest to lowest):
//  1. Environment variables (KARVE_* prefix)
//  2. Configuration file values
//  3. Default values
func Load(path string) (*Config, error) {
	v := viper.New()

	// Set configuration file
	v.SetConfigFile(path)

	// Set default values
	v.SetDefault(KeyPrometheusURL, DefaultPrometheusURL)
	v.SetDefault(KeyLogLevel, DefaultLogLevel)
	v.SetDefault(KeyMetricsBindAddress, DefaultMetricsBindAddress)
	v.SetDefault(KeyHealthProbeBindAddress, DefaultHealthProbeBindAddress)
	v.SetDefault(KeyOverlayUtilizationThreshold, DefaultOverlayUtilizationThreshold)
	v.SetDefault(KeyOverlayWeightReservedInstance, DefaultOverlayWeightReservedInstance)
	v.SetDefault(KeyOverlayWeightEC2InstanceSavingsPlan, DefaultOverlayWeightEC2InstanceSavingsPlan)
	v.SetDefault(KeyOverlayWeightComputeSavingsPlan, DefaultOverlayWeightComputeSavingsPlan)

	// Enable environment variable overrides with KARVE_ prefix
	v.SetEnvPrefix(EnvPrefix)
	_ = v.BindEnv(KeyPrometheusURL, EnvPrometheusURL)
	_ = v.BindEnv(KeyLogLevel, EnvLogLevel)
	_ = v.BindEnv(KeyMetricsBindAddress, EnvMetricsBindAddress)
	_ = v.BindEnv(KeyHealthProbeBindAddress, EnvHealthProbeBindAddress)

	// Read configuration file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	// Unmarshal into Config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Validate checks that the configuration is valid and returns an error if not.
func (c *Config) Validate() error {
	// Validate Prometheus URL
	if c.PrometheusURL == "" {
		return fmt.Errorf("prometheus URL is required")
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if c.LogLevel != "" && !validLogLevels[c.LogLevel] {
		return fmt.Errorf("invalid log level %q, must be one of: debug, info, warn, error", c.LogLevel)
	}

	// Validate overlay management configuration
	if c.OverlayManagement.UtilizationThreshold < 0 || c.OverlayManagement.UtilizationThreshold > 100 {
		return fmt.Errorf(
			"overlay utilization threshold must be between 0 and 100, got %f",
			c.OverlayManagement.UtilizationThreshold,
		)
	}

	// Validate weights are positive
	if c.OverlayManagement.Weights.ReservedInstance < 0 {
		return fmt.Errorf(
			"reserved instance weight must be non-negative, got %d",
			c.OverlayManagement.Weights.ReservedInstance,
		)
	}
	if c.OverlayManagement.Weights.EC2InstanceSavingsPlan < 0 {
		return fmt.Errorf(
			"ec2 instance savings plan weight must be non-negative, got %d",
			c.OverlayManagement.Weights.EC2InstanceSavingsPlan,
		)
	}
	if c.OverlayManagement.Weights.ComputeSavingsPlan < 0 {
		return fmt.Errorf(
			"compute savings plan weight must be non-negative, got %d",
			c.OverlayManagement.Weights.ComputeSavingsPlan,
		)
	}

	return nil
}

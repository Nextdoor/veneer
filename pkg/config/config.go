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
	KeyPrometheusURL          = "prometheusUrl"
	KeyLogLevel               = "logLevel"
	KeyMetricsBindAddress     = "metricsBindAddress"
	KeyHealthProbeBindAddress = "healthProbeBindAddress"
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
	DefaultPrometheusURL          = "http://prometheus:9090"
	DefaultLogLevel               = "info"
	DefaultMetricsBindAddress     = ":8080"
	DefaultHealthProbeBindAddress = ":8081"
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

	return nil
}

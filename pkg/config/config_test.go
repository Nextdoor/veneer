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
`,
			wantErr: false,
			validate: func(t *testing.T, c *Config) {
				if c.PrometheusURL != "http://prometheus:9090" {
					t.Errorf("PrometheusURL = %q, want %q", c.PrometheusURL, "http://prometheus:9090")
				}
				if c.LogLevel != "debug" {
					t.Errorf("LogLevel = %q, want %q", c.LogLevel, "debug")
				}
			},
		},
		{
			name: "minimal valid config",
			configYAML: `
prometheusUrl: "http://prom:9090"
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
`,
			wantErr: true,
		},
		{
			name: "missing prometheus URL uses default",
			configYAML: `
logLevel: "info"
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
			},
			wantErr: false,
		},
		{
			name: "invalid log level",
			config: Config{
				PrometheusURL: "http://prometheus:9090",
				LogLevel:      "trace",
			},
			wantErr: true,
		},
		{
			name: "all valid log levels",
			config: Config{
				PrometheusURL: "http://prometheus:9090",
				LogLevel:      "debug",
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
	configYAML := `prometheusUrl: "http://default:9090"`
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	// Set environment variable
	os.Setenv("KARVE_PROMETHEUS_URL", "http://override:9090")
	defer os.Unsetenv("KARVE_PROMETHEUS_URL")

	// Load config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Environment variable should override file value
	if cfg.PrometheusURL != "http://override:9090" {
		t.Errorf("PrometheusURL = %q, want %q (env var override)", cfg.PrometheusURL, "http://override:9090")
	}
}

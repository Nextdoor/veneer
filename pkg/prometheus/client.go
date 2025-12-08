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

// Package prometheus provides a client for querying Prometheus to retrieve Lumina metrics.
//
// The client abstracts the Prometheus HTTP API and provides typed methods for querying
// specific Lumina metrics needed for cost optimization decisions.
package prometheus

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// Lumina metric name constants.
// TODO: Import these from github.com/nextdoor/lumina/pkg/metrics once
// https://github.com/Nextdoor/lumina/issues/129 is implemented.
const (
	metricSavingsPlanRemainingCapacity  = "savings_plan_remaining_capacity"
	metricSavingsPlanUtilizationPercent = "savings_plan_utilization_percent"
	metricEC2ReservedInstance           = "ec2_reserved_instance"
	metricLuminaDataFreshnessSeconds    = "lumina_data_freshness_seconds"
)

// Lumina metric label name constants.
// TODO: Import these from github.com/nextdoor/lumina/pkg/metrics once
// https://github.com/Nextdoor/lumina/issues/129 is implemented.
const (
	labelInstanceFamily   = "instance_family"
	labelInstanceType     = "instance_type"
	labelType             = "type"
	labelSavingsPlanARN   = "savings_plan_arn"
	labelAccountID        = "account_id"
	labelRegion           = "region"
	labelAvailabilityZone = "availability_zone"
	labelOperatingSystem  = "operating_system"
)

// Savings Plan type constants.
const (
	SavingsPlanTypeCompute     = "compute"
	SavingsPlanTypeEC2Instance = "ec2_instance"
)

// Client is a Prometheus client for querying Lumina metrics.
// It wraps the official Prometheus Go client and provides typed methods
// for the specific metrics Karve needs.
type Client struct {
	api v1.API
}

// NewClient creates a new Prometheus client.
// The url parameter should be the base URL of the Prometheus server
// (e.g., "http://prometheus:9090").
func NewClient(url string) (*Client, error) {
	promClient, err := api.NewClient(api.Config{
		Address: url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus client: %w", err)
	}

	return &Client{
		api: v1.NewAPI(promClient),
	}, nil
}

// SavingsPlanCapacity represents remaining Savings Plan capacity for an instance family.
type SavingsPlanCapacity struct {
	// Type is the Savings Plan type ("ec2_instance" or "compute")
	Type string

	// InstanceFamily is the EC2 instance family (e.g., "m5", "c5")
	InstanceFamily string

	// SavingsPlanARN is the ARN of the Savings Plan
	SavingsPlanARN string

	// AccountID is the AWS account ID
	AccountID string

	// RemainingCapacity is the remaining capacity in $/hour
	RemainingCapacity float64

	// Timestamp is when this metric was recorded
	Timestamp time.Time
}

// ReservedInstance represents a Reserved Instance from Lumina metrics.
type ReservedInstance struct {
	// AccountID is the AWS account ID
	AccountID string

	// Region is the AWS region
	Region string

	// InstanceType is the EC2 instance type (e.g., "m5.xlarge")
	InstanceType string

	// AvailabilityZone is the AZ where the RI is available
	AvailabilityZone string

	// Count is the number of RIs (typically 1 per metric)
	Count int

	// Timestamp is when this metric was recorded
	Timestamp time.Time
}

// QuerySavingsPlanCapacity queries Prometheus for Savings Plan remaining capacity.
// The instanceFamily parameter filters results (e.g., "m5", "c5").
// Pass empty string to get all instance families.
//
// This queries: savings_plan_remaining_capacity{instance_family="$family"}
func (c *Client) QuerySavingsPlanCapacity(ctx context.Context, instanceFamily string) ([]SavingsPlanCapacity, error) {
	// Build query
	var query string
	if instanceFamily != "" {
		query = fmt.Sprintf(`%s{%s="%s"}`, metricSavingsPlanRemainingCapacity, labelInstanceFamily, instanceFamily)
	} else {
		query = metricSavingsPlanRemainingCapacity
	}

	// Execute query
	result, warnings, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("prometheus query failed: %w", err)
	}

	// Log warnings if any
	if len(warnings) > 0 {
		// In production code, use a proper logger here
		// For now, warnings are silently ignored
		_ = warnings
	}

	// Parse results
	vector, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	capacities := make([]SavingsPlanCapacity, 0, len(vector))
	for _, sample := range vector {
		capacity := SavingsPlanCapacity{
			Type:              string(sample.Metric[labelType]),
			InstanceFamily:    string(sample.Metric[labelInstanceFamily]),
			SavingsPlanARN:    string(sample.Metric[labelSavingsPlanARN]),
			AccountID:         string(sample.Metric[labelAccountID]),
			RemainingCapacity: float64(sample.Value),
			Timestamp:         sample.Timestamp.Time(),
		}
		capacities = append(capacities, capacity)
	}

	return capacities, nil
}

// QueryReservedInstances queries Prometheus for Reserved Instances.
// The instanceType parameter filters results (e.g., "m5.xlarge").
// Pass empty string to get all instance types.
//
// This queries: ec2_reserved_instance{instance_type="$type"}
func (c *Client) QueryReservedInstances(ctx context.Context, instanceType string) ([]ReservedInstance, error) {
	// Build query
	var query string
	if instanceType != "" {
		query = fmt.Sprintf(`%s{%s="%s"}`, metricEC2ReservedInstance, labelInstanceType, instanceType)
	} else {
		query = metricEC2ReservedInstance
	}

	// Execute query
	result, warnings, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("prometheus query failed: %w", err)
	}

	// Log warnings if any
	if len(warnings) > 0 {
		_ = warnings
	}

	// Parse results
	vector, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	ris := make([]ReservedInstance, 0, len(vector))
	for _, sample := range vector {
		// Parse count (the metric value represents the count)
		count := int(sample.Value)

		ri := ReservedInstance{
			AccountID:        string(sample.Metric[labelAccountID]),
			Region:           string(sample.Metric[labelRegion]),
			InstanceType:     string(sample.Metric[labelInstanceType]),
			AvailabilityZone: string(sample.Metric[labelAvailabilityZone]),
			Count:            count,
			Timestamp:        sample.Timestamp.Time(),
		}
		ris = append(ris, ri)
	}

	return ris, nil
}

// buildInstanceTypeQuery builds a Prometheus query with optional instance_type filter.
func buildInstanceTypeQuery(metricName, instanceType string) string {
	if instanceType != "" {
		return fmt.Sprintf(`%s{instance_type="%s"}`, metricName, instanceType)
	}
	return metricName
}

// executeQuery executes a Prometheus query and returns the vector result.
func (c *Client) executeQuery(ctx context.Context, query string) (model.Vector, error) {
	result, warnings, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("prometheus query failed: %w", err)
	}

	if len(warnings) > 0 {
		_ = warnings
	}

	vector, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	return vector, nil
}

// SpotPrice represents current spot pricing from Lumina metrics.
type SpotPrice struct {
	// InstanceType is the EC2 instance type
	InstanceType string

	// Region is the AWS region
	Region string

	// AvailabilityZone is the specific AZ
	AvailabilityZone string

	// Price is the current spot price in $/hour
	Price float64

	// Timestamp is when this metric was recorded
	Timestamp time.Time
}

// QuerySpotPrice queries Prometheus for current spot prices.
// The instanceType parameter filters results (e.g., "m5.xlarge").
// Pass empty string to get all instance types.
//
// This queries: ec2_spot_price{instance_type="$type"}
func (c *Client) QuerySpotPrice(ctx context.Context, instanceType string) ([]SpotPrice, error) {
	query := buildInstanceTypeQuery("ec2_spot_price", instanceType)
	vector, err := c.executeQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	prices := make([]SpotPrice, 0, len(vector))
	for _, sample := range vector {
		price := SpotPrice{
			InstanceType:     string(sample.Metric["instance_type"]),
			Region:           string(sample.Metric["region"]),
			AvailabilityZone: string(sample.Metric["availability_zone"]),
			Price:            float64(sample.Value),
			Timestamp:        sample.Timestamp.Time(),
		}
		prices = append(prices, price)
	}

	return prices, nil
}

// OnDemandPrice represents on-demand pricing from Lumina metrics.
type OnDemandPrice struct {
	// InstanceType is the EC2 instance type
	InstanceType string

	// Region is the AWS region
	Region string

	// OperatingSystem is the OS (e.g., "Linux", "Windows")
	OperatingSystem string

	// Price is the on-demand price in $/hour
	Price float64

	// Timestamp is when this metric was recorded
	Timestamp time.Time
}

// QueryOnDemandPrice queries Prometheus for on-demand prices.
// The instanceType parameter filters results (e.g., "m5.xlarge").
// Pass empty string to get all instance types.
//
// This queries: ec2_ondemand_price{instance_type="$type"}
func (c *Client) QueryOnDemandPrice(ctx context.Context, instanceType string) ([]OnDemandPrice, error) {
	query := buildInstanceTypeQuery("ec2_ondemand_price", instanceType)
	vector, err := c.executeQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	prices := make([]OnDemandPrice, 0, len(vector))
	for _, sample := range vector {
		price := OnDemandPrice{
			InstanceType:    string(sample.Metric["instance_type"]),
			Region:          string(sample.Metric["region"]),
			OperatingSystem: string(sample.Metric["operating_system"]),
			Price:           float64(sample.Value),
			Timestamp:       sample.Timestamp.Time(),
		}
		prices = append(prices, price)
	}

	return prices, nil
}

// DataFreshness queries the lumina_data_freshness_seconds metric to check how old
// Lumina's data is. Returns the age in seconds, or an error if the metric is not available.
//
// This is useful for determining if Lumina's data is stale and cost decisions
// should be delayed until fresh data is available.
func (c *Client) DataFreshness(ctx context.Context) (float64, error) {
	query := metricLuminaDataFreshnessSeconds

	result, warnings, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("prometheus query failed: %w", err)
	}

	if len(warnings) > 0 {
		_ = warnings
	}

	vector, ok := result.(model.Vector)
	if !ok {
		return 0, fmt.Errorf("unexpected result type: %T", result)
	}

	if len(vector) == 0 {
		return 0, fmt.Errorf("no data freshness metric available")
	}

	// Return the first sample (should only be one)
	return float64(vector[0].Value), nil
}

// SavingsPlanUtilization represents current utilization of a Savings Plan.
// This is critical for overlay lifecycle decisions (create/delete based on capacity thresholds).
type SavingsPlanUtilization struct {
	// Type is the Savings Plan type ("ec2_instance" or "compute")
	Type string

	// InstanceFamily is the EC2 instance family (e.g., "m5", "c5")
	// Empty for Compute SPs
	InstanceFamily string

	// Region is the AWS region (empty for Compute SPs)
	Region string

	// SavingsPlanARN is the ARN of the Savings Plan
	SavingsPlanARN string

	// AccountID is the AWS account ID
	AccountID string

	// UtilizationPercent is the current utilization percentage (0-100+)
	// Can exceed 100% if over-committed (spillover to on-demand)
	UtilizationPercent float64

	// Timestamp is when this metric was recorded
	Timestamp time.Time
}

// QuerySavingsPlanUtilization queries Prometheus for Savings Plan utilization percentages.
// This is critical for determining when to create/delete NodeOverlays based on capacity thresholds.
//
// The spType parameter filters by Savings Plan type ("compute" or "ec2_instance").
// Pass empty string to get all types.
//
// This queries: savings_plan_utilization_percent{type="$spType"}
func (c *Client) QuerySavingsPlanUtilization(ctx context.Context, spType string) ([]SavingsPlanUtilization, error) {
	// Build query
	var query string
	if spType != "" {
		query = fmt.Sprintf(`%s{%s="%s"}`, metricSavingsPlanUtilizationPercent, labelType, spType)
	} else {
		query = metricSavingsPlanUtilizationPercent
	}

	// Execute query
	result, warnings, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("prometheus query failed: %w", err)
	}

	if len(warnings) > 0 {
		_ = warnings
	}

	// Parse results
	vector, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	utilizations := make([]SavingsPlanUtilization, 0, len(vector))
	for _, sample := range vector {
		util := SavingsPlanUtilization{
			Type:               string(sample.Metric[labelType]),
			InstanceFamily:     string(sample.Metric[labelInstanceFamily]),
			Region:             string(sample.Metric[labelRegion]),
			SavingsPlanARN:     string(sample.Metric[labelSavingsPlanARN]),
			AccountID:          string(sample.Metric[labelAccountID]),
			UtilizationPercent: float64(sample.Value),
			Timestamp:          sample.Timestamp.Time(),
		}
		utilizations = append(utilizations, util)
	}

	return utilizations, nil
}

// QueryRaw executes a raw PromQL query and returns the result as a string.
// This is useful for debugging or custom queries not covered by typed methods.
//
// The result is formatted as: metric_name{labels} value
func (c *Client) QueryRaw(ctx context.Context, query string) (string, error) {
	result, warnings, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return "", fmt.Errorf("prometheus query failed: %w", err)
	}

	if len(warnings) > 0 {
		_ = warnings
	}

	// Format result as string
	return result.String(), nil
}

// ParseFloat64 is a helper to safely parse Prometheus metric values.
// Prometheus returns values as model.SampleValue which is a float64 alias.
func ParseFloat64(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

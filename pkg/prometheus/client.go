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

	"github.com/go-logr/logr"
	luminametrics "github.com/nextdoor/lumina/pkg/metrics"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// Re-export Lumina metric and label constants for convenience.
// These constants provide type-safe, compile-time checked access to
// Lumina metric names and labels. See github.com/nextdoor/lumina/pkg/metrics
// for full documentation.
const (
	// Metric names
	metricSavingsPlanRemainingCapacity  = luminametrics.MetricSavingsPlanRemainingCapacity
	metricSavingsPlanUtilizationPercent = luminametrics.MetricSavingsPlanUtilizationPercent
	metricSavingsPlanHourlyCommitment   = luminametrics.MetricSavingsPlanHourlyCommitment
	metricEC2ReservedInstance           = luminametrics.MetricEC2ReservedInstance
	metricLuminaDataFreshnessSeconds    = luminametrics.MetricLuminaDataFreshnessSeconds

	// Label names
	labelInstanceFamily   = luminametrics.LabelInstanceFamily
	labelInstanceType     = luminametrics.LabelInstanceType
	labelType             = luminametrics.LabelType
	labelSavingsPlanARN   = luminametrics.LabelSavingsPlanARN
	labelAccountID        = luminametrics.LabelAccountID
	labelRegion           = luminametrics.LabelRegion
	labelAvailabilityZone = luminametrics.LabelAvailabilityZone
	labelOperatingSystem  = "operating_system" // Not exported by Lumina yet
)

// Savings Plan type constants.
const (
	SavingsPlanTypeCompute     = "compute"
	SavingsPlanTypeEC2Instance = "ec2_instance"
)

// Client is a Prometheus client for querying Lumina metrics.
// It wraps the official Prometheus Go client and provides typed methods
// for the specific metrics Karve needs.
//
// The client is scoped to a specific AWS account and region to prevent
// creating NodeOverlays for capacity that won't apply to this cluster.
type Client struct {
	api       v1.API
	accountID string      // AWS account ID to filter queries (prevents cross-account queries)
	region    string      // AWS region to filter queries (prevents cross-region queries)
	logger    logr.Logger // Logger for debugging query execution
}

// NewClient creates a new Prometheus client scoped to a specific AWS account and region.
//
// The url parameter should be the base URL of the Prometheus server (e.g., "http://prometheus:9090").
// The accountID and region parameters are used to filter all queries to only return metrics
// for capacity in this cluster's account and region.
// The logger parameter is used for debugging query execution (pass logr.Discard() to disable).
//
// Why scoping is critical:
// Lumina monitors multiple AWS accounts and regions, but this Karve instance runs in ONE cluster
// in ONE account and ONE region. Without filtering, we would create NodeOverlays for RIs/SPs from
// other accounts/regions, causing Karpenter to launch on-demand instances that won't actually
// receive the pre-paid discount.
func NewClient(url, accountID, region string, logger logr.Logger) (*Client, error) {
	promClient, err := api.NewClient(api.Config{
		Address: url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus client: %w", err)
	}

	return &Client{
		api:       v1.NewAPI(promClient),
		accountID: accountID,
		region:    region,
		logger:    logger,
	}, nil
}

// SavingsPlanCapacity represents remaining Savings Plan capacity for an instance family.
type SavingsPlanCapacity struct {
	// Type is the Savings Plan type ("ec2_instance" or "compute")
	Type string

	// InstanceFamily is the EC2 instance family (e.g., "m5", "c5").
	// Optional: empty string for Compute SPs (which apply globally to all families).
	// Only populated for EC2 Instance SPs.
	InstanceFamily string

	// Region is the AWS region.
	// Optional: "all" for Compute SPs (which apply globally to all regions).
	// Only populated with specific region for EC2 Instance SPs.
	Region string

	// SavingsPlanARN is the ARN of the Savings Plan
	SavingsPlanARN string

	// AccountID is the AWS account ID
	AccountID string

	// RemainingCapacity is the remaining capacity in $/hour
	RemainingCapacity float64

	// HourlyCommitment is the total hourly commitment in $/hour
	HourlyCommitment float64

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

// QuerySavingsPlanCapacity queries Prometheus for Savings Plan remaining capacity and hourly commitment.
// The instanceFamily parameter filters results (e.g., "m5", "c5").
// Pass empty string to get all instance families (includes both EC2 Instance and Compute SPs).
//
// This queries both savings_plan_remaining_capacity and savings_plan_hourly_commitment metrics.
//
// Important: We use savings_plan_hourly_commitment as the PRIMARY source because it has
// instance_family and region labels, while savings_plan_remaining_capacity does not.
// We then join in the remaining capacity data using the Savings Plan ARN as the correlation key.
//
// Filtering behavior (see https://github.com/Nextdoor/lumina/blob/main/ALGORITHM.md):
// - Compute SPs: GLOBAL - apply to ANY instance family in ANY region. NOT filtered by account_id or region.
// - EC2 Instance SPs: REGIONAL - apply to specific instance family in specific region. Filtered by account_id AND region.
//
// When instanceFamily is specified: Only EC2 Instance SPs for that family+account+region are returned.
// When instanceFamily is empty: Both Compute SPs (global) and EC2 Instance SPs (account+region) are returned.
func (c *Client) QuerySavingsPlanCapacity(ctx context.Context, instanceFamily string) ([]SavingsPlanCapacity, error) {
	// Capture query time once at the start to ensure consistency across both queries
	queryTime := time.Now()

	// Build queries for both metrics
	// IMPORTANT: Compute Savings Plans are GLOBAL and should NOT be filtered by account_id or region.
	// EC2 Instance Savings Plans are scoped to account+region and should be filtered.
	//
	// Note: Only the commitment metric has instance_family and region labels.
	// The remaining capacity metric only has account_id and type labels.
	var commitmentQuery, remainingQuery string

	if instanceFamily != "" {
		// Specific family: get EC2 Instance SPs for this region in this family
		// This is a regional/family-based savings plan, so we SHOULD filter by account_id and region
		commitmentQuery = fmt.Sprintf(`%s{%s="%s", %s="%s", %s="%s", %s="%s"}`,
			metricSavingsPlanHourlyCommitment,
			labelType, SavingsPlanTypeEC2Instance,
			labelAccountID, c.accountID,
			labelRegion, c.region,
			labelInstanceFamily, instanceFamily)

		// For remaining capacity, filter to EC2 Instance SPs for this account+region
		remainingQuery = fmt.Sprintf(`%s{%s="%s", %s="%s"}`,
			metricSavingsPlanRemainingCapacity,
			labelType, SavingsPlanTypeEC2Instance,
			labelAccountID, c.accountID)
	} else {
		// All families: get BOTH Compute SPs (global, no filters) AND EC2 Instance SPs (account+region)
		// We use sum() to aggregate multiple queries with the 'or' operator
		commitmentQuery = fmt.Sprintf(`%s{%s="%s"} or %s{%s="%s", %s="%s", %s="%s"}`,
			// Compute SPs: global, no account/region filters
			metricSavingsPlanHourlyCommitment,
			labelType, SavingsPlanTypeCompute,
			// EC2 Instance SPs: filtered by account+region
			metricSavingsPlanHourlyCommitment,
			labelType, SavingsPlanTypeEC2Instance,
			labelAccountID, c.accountID,
			labelRegion, c.region)

		// For remaining capacity: get both types (no filters for Compute, account filter for EC2)
		remainingQuery = fmt.Sprintf(`%s{%s="%s"} or %s{%s="%s", %s="%s"}`,
			// Compute SPs: global, no filters
			metricSavingsPlanRemainingCapacity,
			labelType, SavingsPlanTypeCompute,
			// EC2 Instance SPs: filter by account
			metricSavingsPlanRemainingCapacity,
			labelType, SavingsPlanTypeEC2Instance,
			labelAccountID, c.accountID)
	}

	// Log the queries for debugging
	c.logger.V(1).Info("Executing Prometheus queries for Savings Plan capacity",
		"commitment_query", commitmentQuery,
		"remaining_query", remainingQuery,
		"account_id", c.accountID,
		"region", c.region)

	// Execute hourly commitment query first (this is our PRIMARY data source)
	commitmentResult, warnings, err := c.api.Query(ctx, commitmentQuery, queryTime)
	if err != nil {
		return nil, fmt.Errorf("prometheus query for hourly commitment failed: %w", err)
	}
	if len(warnings) > 0 {
		_ = warnings
	}

	// Execute remaining capacity query
	remainingResult, warnings, err := c.api.Query(ctx, remainingQuery, queryTime)
	if err != nil {
		return nil, fmt.Errorf("prometheus query for remaining capacity failed: %w", err)
	}
	if len(warnings) > 0 {
		_ = warnings
	}

	// Parse hourly commitment results (PRIMARY source)
	commitmentVector, ok := commitmentResult.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected hourly commitment result type: %T", commitmentResult)
	}

	// Parse remaining capacity results
	remainingVector, ok := remainingResult.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected remaining capacity result type: %T", remainingResult)
	}

	// Build a map of ARN -> remaining capacity for fast lookup
	remainingByARN := make(map[string]float64, len(remainingVector))
	for _, sample := range remainingVector {
		arn := string(sample.Metric[labelSavingsPlanARN])
		remainingByARN[arn] = float64(sample.Value)
	}

	// Combine the data using hourly commitment as the primary source
	// This ensures we get the correct instance_family and region labels from the commitment metric
	capacities := make([]SavingsPlanCapacity, 0, len(commitmentVector))
	for _, sample := range commitmentVector {
		arn := string(sample.Metric[labelSavingsPlanARN])
		capacity := SavingsPlanCapacity{
			Type:              string(sample.Metric[labelType]),
			InstanceFamily:    string(sample.Metric[labelInstanceFamily]),
			Region:            string(sample.Metric[labelRegion]),
			SavingsPlanARN:    arn,
			AccountID:         string(sample.Metric[labelAccountID]),
			RemainingCapacity: remainingByARN[arn], // Lookup remaining capacity by ARN
			HourlyCommitment:  float64(sample.Value),
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
// The client is scoped to a specific account and region, so only RIs from this cluster's
// account/region are returned.
func (c *Client) QueryReservedInstances(ctx context.Context, instanceType string) ([]ReservedInstance, error) {
	// Build query with account/region filtering
	var query string
	if instanceType != "" {
		query = fmt.Sprintf(`%s{%s="%s", %s="%s", %s="%s"}`,
			metricEC2ReservedInstance,
			labelAccountID, c.accountID,
			labelRegion, c.region,
			labelInstanceType, instanceType)
	} else {
		query = fmt.Sprintf(`%s{%s="%s", %s="%s"}`,
			metricEC2ReservedInstance,
			labelAccountID, c.accountID,
			labelRegion, c.region)
	}

	// Log the query for debugging
	c.logger.V(1).Info("Executing Prometheus query for Reserved Instances",
		"query", query,
		"account_id", c.accountID,
		"region", c.region)

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

	// InstanceFamily is the EC2 instance family (e.g., "m5", "c5").
	// Optional: empty string for Compute SPs (which apply globally to all families).
	// Only populated for EC2 Instance SPs.
	InstanceFamily string

	// Region is the AWS region.
	// Optional: empty string for Compute SPs (which apply globally to all regions).
	// Only populated for EC2 Instance SPs.
	Region string

	// SavingsPlanARN is the ARN of the Savings Plan
	SavingsPlanARN string

	// AccountID is the AWS account ID
	AccountID string

	// UtilizationPercent is the current utilization percentage (0-100+).
	// Can exceed 100% if over-committed (spillover to on-demand rates).
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
// The client is scoped to a specific account, so only SPs from this cluster's account are returned.
// Note: We don't filter by region here because utilization metrics don't have region labels.
func (c *Client) QuerySavingsPlanUtilization(ctx context.Context, spType string) ([]SavingsPlanUtilization, error) {
	// Build query with account filtering (utilization metric doesn't have region label)
	var query string
	if spType != "" {
		query = fmt.Sprintf(`%s{%s="%s", %s="%s"}`,
			metricSavingsPlanUtilizationPercent,
			labelAccountID, c.accountID,
			labelType, spType)
	} else {
		query = fmt.Sprintf(`%s{%s="%s"}`,
			metricSavingsPlanUtilizationPercent,
			labelAccountID, c.accountID)
	}

	// Log the query for debugging
	c.logger.V(1).Info("Executing Prometheus query for Savings Plan utilization",
		"query", query,
		"account_id", c.accountID)

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

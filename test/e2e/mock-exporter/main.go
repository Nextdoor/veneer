// Mock Lumina metrics exporter for E2E tests
package main

import (
	"fmt"
	"log"
	"net/http"
)

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	// Mock Lumina metrics in Prometheus format
	// Note: data_freshness requires account_id and data_type labels to match queries
	metrics := `# HELP lumina_data_freshness_seconds Time since last successful AWS API refresh
# TYPE lumina_data_freshness_seconds gauge
lumina_data_freshness_seconds{account_id="123456789012",data_type="savings_plans"} 30
lumina_data_freshness_seconds{account_id="123456789012",data_type="reserved_instances"} 30
lumina_data_freshness_seconds{account_id="123456789012",data_type="ec2_instances"} 30
lumina_data_freshness_seconds{account_id="123456789012",data_type="spot-pricing"} 30
lumina_data_freshness_seconds{account_id="123456789012",data_type="pricing"} 30

# HELP lumina_savings_plan_capacity_hours Remaining Savings Plan capacity in normalized hours
# TYPE lumina_savings_plan_capacity_hours gauge
lumina_savings_plan_capacity_hours{instance_family="m5",plan_type="Compute"} 100.5
lumina_savings_plan_capacity_hours{instance_family="c5",plan_type="Compute"} 50.25
lumina_savings_plan_capacity_hours{instance_family="r5",plan_type="EC2Instance"} 75.0

# HELP lumina_reserved_instance_count Number of active Reserved Instances
# TYPE lumina_reserved_instance_count gauge
lumina_reserved_instance_count{instance_type="m5.large",availability_zone="us-west-2a"} 10
lumina_reserved_instance_count{instance_type="m5.xlarge",availability_zone="us-west-2b"} 5
lumina_reserved_instance_count{instance_type="c5.2xlarge",availability_zone="us-west-2a"} 8

# HELP lumina_spot_price_usd Current Spot price in USD per hour
# TYPE lumina_spot_price_usd gauge
lumina_spot_price_usd{instance_type="m5.large",availability_zone="us-west-2a"} 0.045
lumina_spot_price_usd{instance_type="m5.xlarge",availability_zone="us-west-2b"} 0.089
lumina_spot_price_usd{instance_type="c5.2xlarge",availability_zone="us-west-2a"} 0.125

# HELP lumina_ondemand_price_usd On-Demand price in USD per hour
# TYPE lumina_ondemand_price_usd gauge
lumina_ondemand_price_usd{instance_type="m5.large"} 0.096
lumina_ondemand_price_usd{instance_type="m5.xlarge"} 0.192
lumina_ondemand_price_usd{instance_type="c5.2xlarge"} 0.34
`

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprint(w, metrics)
}

func main() {
	http.HandleFunc("/metrics", metricsHandler)

	log.Println("Mock Lumina exporter listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

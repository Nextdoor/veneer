//go:build e2e
// +build e2e

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

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/nextdoor/karve/test/utils"
)

const (
	namespace = "karve-system"
)

var (
	// projectImage is the name of the image which will be built and loaded
	// with the code source changes to be tested.
	projectImage = "example.com/karve:v0.0.1"
)

// TestE2E runs the end-to-end (e2e) test suite for the project.
// These tests execute in an isolated, temporary environment to validate
// project changes with the purpose of being used in CI jobs.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting Karve integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("building the manager(Operator) image")
	cmd := exec.Command("docker", "build", "-t", projectImage, ".")
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build Docker image")

	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load image to Kind cluster")

	By("creating manager namespace")
	cmd = exec.Command("kubectl", "create", "ns", namespace, "--dry-run=client", "-o", "yaml")
	yamlOutput, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to generate namespace YAML")

	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(yamlOutput)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create namespace")

	By("deploying a mock Prometheus server for testing")
	// Deploy a simple HTTP server that returns empty metrics for now
	// In a real setup, you'd deploy the actual Lumina or a mock with test data
	mockPrometheusYAML := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: mock-prometheus-config
  namespace: ` + namespace + `
data:
  response.json: |
    {"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"lumina_data_freshness_seconds"},"value":[1640000000,"30"]}]}}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mock-prometheus
  namespace: ` + namespace + `
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mock-prometheus
  template:
    metadata:
      labels:
        app: mock-prometheus
    spec:
      containers:
      - name: mock-server
        image: python:3.11-alpine
        command: ["python3", "-m", "http.server", "9090"]
        ports:
        - containerPort: 9090
---
apiVersion: v1
kind: Service
metadata:
  name: prometheus
  namespace: ` + namespace + `
spec:
  selector:
    app: mock-prometheus
  ports:
  - port: 9090
    targetPort: 9090
`
	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(mockPrometheusYAML)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to deploy mock Prometheus")

	By("creating Karve ConfigMap")
	configYAML := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: karve-config
  namespace: ` + namespace + `
data:
  config.yaml: |
    prometheusURL: "http://prometheus.` + namespace + `.svc.cluster.local:9090"
    logLevel: "debug"
`
	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(configYAML)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create ConfigMap")

	By("deploying the Karve controller-manager")
	deploymentYAML := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: karve-controller-manager
  namespace: ` + namespace + `
spec:
  replicas: 1
  selector:
    matchLabels:
      control-plane: controller-manager
  template:
    metadata:
      labels:
        control-plane: controller-manager
    spec:
      containers:
      - name: manager
        image: ` + projectImage + `
        imagePullPolicy: IfNotPresent
        args:
        - --config=/etc/karve/config.yaml
        - --leader-elect=false
        volumeMounts:
        - name: config
          mountPath: /etc/karve
      volumes:
      - name: config
        configMap:
          name: karve-config
      serviceAccountName: default
`
	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(deploymentYAML)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to deploy controller-manager")

	By("waiting for controller-manager deployment to be ready")
	cmd = exec.Command("kubectl", "rollout", "status",
		"deployment/karve-controller-manager",
		"-n", namespace,
		"--timeout=60s")
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Controller-manager deployment did not become ready")
})

var _ = AfterSuite(func() {
	By("fetching controller logs before teardown")
	client, err := NewLogsClient(namespace)
	if err == nil {
		logs, err := client.GetPodLogsByLabel(context.Background(), "control-plane=controller-manager", nil)
		if err == nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "\n========== Controller Manager Logs ==========\n%s\n========================================\n", logs)
		}
	}

	By("undeploying the controller-manager")
	cmd := exec.Command("kubectl", "delete", "deployment", "karve-controller-manager", "-n", namespace, "--ignore-not-found=true")
	_, _ = utils.Run(cmd)

	By("removing mock Prometheus")
	cmd = exec.Command("kubectl", "delete", "deployment,service,configmap", "-l", "app=mock-prometheus", "-n", namespace, "--ignore-not-found=true")
	_, _ = utils.Run(cmd)

	By("removing manager namespace")
	cmd = exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found=true")
	_, _ = utils.Run(cmd)
})

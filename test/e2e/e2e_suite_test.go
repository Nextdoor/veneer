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
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

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
	ctx := context.Background()

	By("building the manager(Operator) image")
	cmd := exec.Command("docker", "build", "-t", projectImage, ".")
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build Docker image")

	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load image to Kind cluster")

	By("creating manager namespace")
	client, err := NewResourceClient("")
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create resource client")

	err = client.CreateNamespace(ctx, namespace)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create namespace")

	// Update client to use the new namespace
	client.namespace = namespace

	By("deploying a mock Prometheus server for testing")
	// Create ConfigMap with mock data
	mockData := map[string]string{
		"response.json": `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"lumina_data_freshness_seconds"},"value":[1640000000,"30"]}]}}`,
	}
	err = client.CreateConfigMapFromYAML(ctx, "mock-prometheus-config", mockData)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create mock Prometheus ConfigMap")

	// Create mock Prometheus deployment
	replicas := int32(1)
	mockPrometheusDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mock-prometheus",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "mock-prometheus"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "mock-prometheus"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "mock-server",
							Image: "python:3.11-alpine",
							Command: []string{"python3", "-m", "http.server", "9090"},
							Ports: []corev1.ContainerPort{
								{ContainerPort: 9090},
							},
						},
					},
				},
			},
		},
	}
	err = client.CreateDeployment(ctx, mockPrometheusDeployment)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create mock Prometheus deployment")

	// Create Service for mock Prometheus
	mockPrometheusService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "mock-prometheus"},
			Ports: []corev1.ServicePort{
				{
					Port:       9090,
					TargetPort: intstr.FromInt(9090),
				},
			},
		},
	}
	err = client.CreateService(ctx, mockPrometheusService)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create mock Prometheus service")

	By("creating Karve ConfigMap")
	karveConfig := map[string]string{
		"config.yaml": fmt.Sprintf("prometheusURL: \"http://prometheus.%s.svc.cluster.local:9090\"\nlogLevel: \"debug\"\n", namespace),
	}
	err = client.CreateConfigMapFromYAML(ctx, "karve-config", karveConfig)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create Karve ConfigMap")

	By("deploying the Karve controller-manager")
	karveDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "karve-controller-manager",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"control-plane": "controller-manager"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"control-plane": "controller-manager"},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "default",
					Containers: []corev1.Container{
						{
							Name:            "manager",
							Image:           projectImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args:            []string{"--config=/etc/karve/config.yaml", "--leader-elect=false"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/karve",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "karve-config",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	err = client.CreateDeployment(ctx, karveDeployment)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create Karve controller-manager deployment")

	By("waiting for controller-manager deployment to be ready")
	Eventually(func(g Gomega) {
		err := client.WaitForDeploymentReady(ctx, "karve-controller-manager")
		g.Expect(err).NotTo(HaveOccurred())
	}, 60*time.Second, 2*time.Second).Should(Succeed())
})

var _ = AfterSuite(func() {
	ctx := context.Background()

	By("fetching controller logs before teardown")
	logsClient, err := NewLogsClient(namespace)
	if err == nil {
		logs, err := logsClient.GetPodLogsByLabel(ctx, "control-plane=controller-manager", nil)
		if err == nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "\n========== Controller Manager Logs ==========\n%s\n========================================\n", logs)
		}
	}

	By("undeploying resources")
	client, err := NewResourceClient(namespace)
	if err == nil {
		_ = client.DeleteDeployment(ctx, "karve-controller-manager")
		_ = client.DeleteDeployment(ctx, "mock-prometheus")
		_ = client.DeleteService(ctx, "prometheus")
		_ = client.DeleteConfigMap(ctx, "karve-config")
		_ = client.DeleteConfigMap(ctx, "mock-prometheus-config")
	}

	By("removing manager namespace")
	if err == nil {
		_ = client.DeleteNamespace(ctx, namespace)
	}
})

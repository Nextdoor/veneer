//go:build e2e
// +build e2e

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

	"github.com/nextdoor/veneer/test/utils"
)

const (
	namespace = "veneer-system"
)

var (
	// projectImage is the name of the image which will be built and loaded
	// with the code source changes to be tested.
	projectImage = "example.com/veneer:v0.0.1"
)

// TestE2E runs the end-to-end (e2e) test suite for the project.
// These tests execute in an isolated, temporary environment to validate
// project changes with the purpose of being used in CI jobs.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting Veneer integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	ctx := context.Background()

	By("building the manager(Operator) image")
	cmd := exec.Command("docker", "build", "-t", projectImage, ".")
	cmd.Dir = "../.." // Set working directory to project root
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build Docker image")

	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load image to Kind cluster")

	By("building the mock Lumina exporter image")
	cmd = exec.Command("docker", "build", "-t", "mock-lumina-exporter:test", "-f", "test/e2e/mock-exporter/Dockerfile", "test/e2e/mock-exporter")
	cmd.Dir = "../.."
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build mock exporter image")

	By("loading the mock exporter image on Kind")
	err = utils.LoadImageToKindClusterWithName("mock-lumina-exporter:test")
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load mock exporter image to Kind cluster")

	By("creating manager namespace")
	client, err := NewResourceClient("")
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create resource client")

	err = client.CreateNamespace(ctx, namespace)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create namespace")

	// Update client to use the new namespace
	client.namespace = namespace

	By("deploying mock Lumina exporter")
	replicas := int32(1)
	mockExporterDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mock-lumina-exporter",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "mock-lumina-exporter"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "mock-lumina-exporter"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "exporter",
							Image:           "mock-lumina-exporter:test",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8080},
							},
						},
					},
				},
			},
		},
	}
	err = client.CreateDeployment(ctx, mockExporterDeployment)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create mock exporter deployment")

	mockExporterService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mock-lumina-exporter",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "mock-lumina-exporter"},
			Ports: []corev1.ServicePort{
				{
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
				},
			},
		},
	}
	err = client.CreateService(ctx, mockExporterService)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create mock exporter service")

	By("deploying Prometheus server")
	// Create Prometheus ConfigMap
	prometheusConfig := map[string]string{
		"prometheus.yml": `global:
  scrape_interval: 5s
  evaluation_interval: 5s

scrape_configs:
  - job_name: 'lumina'
    static_configs:
      - targets: ['mock-lumina-exporter:8080']
`,
	}
	err = client.CreateConfigMapFromYAML(ctx, "prometheus-config", prometheusConfig)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create Prometheus ConfigMap")

	prometheusDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "prometheus"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "prometheus"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "prometheus",
							Image: "prom/prometheus:latest",
							Args: []string{
								"--config.file=/etc/prometheus/prometheus.yml",
								"--storage.tsdb.path=/prometheus",
								"--web.console.libraries=/usr/share/prometheus/console_libraries",
								"--web.console.templates=/usr/share/prometheus/consoles",
							},
							Ports: []corev1.ContainerPort{
								{ContainerPort: 9090},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/prometheus",
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
										Name: "prometheus-config",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	err = client.CreateDeployment(ctx, prometheusDeployment)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create Prometheus deployment")

	prometheusService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "prometheus"},
			Ports: []corev1.ServicePort{
				{
					Port:       9090,
					TargetPort: intstr.FromInt(9090),
				},
			},
		},
	}
	err = client.CreateService(ctx, prometheusService)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create Prometheus service")

	By("waiting for mock exporter deployment to be ready")
	Eventually(func(g Gomega) {
		err := client.WaitForDeploymentReady(ctx, "mock-lumina-exporter")
		g.Expect(err).NotTo(HaveOccurred())
	}, 60*time.Second, 2*time.Second).Should(Succeed())

	By("waiting for Prometheus deployment to be ready")
	Eventually(func(g Gomega) {
		err := client.WaitForDeploymentReady(ctx, "prometheus")
		g.Expect(err).NotTo(HaveOccurred())
	}, 60*time.Second, 2*time.Second).Should(Succeed())

	By("waiting for Prometheus to scrape metrics from mock exporter")
	// Give Prometheus time to scrape the mock exporter (scrape interval is 5s, wait 15s to be safe)
	time.Sleep(15 * time.Second)

	By("creating Veneer ConfigMap")
	veneerConfig := map[string]string{
		"config.yaml": fmt.Sprintf(`prometheusURL: "http://prometheus.%s.svc.cluster.local:9090"
logLevel: "debug"
aws:
  accountID: "123456789012"
  region: "us-west-2"
`, namespace),
	}
	err = client.CreateConfigMapFromYAML(ctx, "veneer-config", veneerConfig)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create Veneer ConfigMap")

	By("deploying the Veneer controller-manager")
	veneerDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "veneer-controller-manager",
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
							Args:            []string{"--config=/etc/veneer/config.yaml", "--leader-elect=false"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/veneer",
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
										Name: "veneer-config",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	err = client.CreateDeployment(ctx, veneerDeployment)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create Veneer controller-manager deployment")

	By("waiting for controller-manager deployment to be ready")
	Eventually(func(g Gomega) {
		err := client.WaitForDeploymentReady(ctx, "veneer-controller-manager")
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
		_ = client.DeleteDeployment(ctx, "veneer-controller-manager")
		_ = client.DeleteDeployment(ctx, "prometheus")
		_ = client.DeleteDeployment(ctx, "mock-lumina-exporter")
		_ = client.DeleteService(ctx, "prometheus")
		_ = client.DeleteService(ctx, "mock-lumina-exporter")
		_ = client.DeleteConfigMap(ctx, "veneer-config")
		_ = client.DeleteConfigMap(ctx, "prometheus-config")
	}

	By("removing manager namespace")
	if err == nil {
		_ = client.DeleteNamespace(ctx, namespace)
	}
})

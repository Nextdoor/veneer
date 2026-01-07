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
	"bytes"
	"context"
	"fmt"
	"io"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// LogsClient provides a clean interface to fetch logs from pods
// using the native Kubernetes client instead of kubectl commands.
type LogsClient struct {
	namespace  string
	clientset  *kubernetes.Clientset
	restConfig *rest.Config
}

// NewLogsClient creates a new logs client.
func NewLogsClient(namespace string) (*LogsClient, error) {
	// Load the kubeconfig from the default location or KUBECONFIG env var
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &LogsClient{
		namespace:  namespace,
		clientset:  clientset,
		restConfig: config,
	}, nil
}

// GetPodLogs retrieves logs from a specific pod by name.
//
// Options:
// - tailLines: Number of lines from the end of the logs (nil = all logs)
// - container: Specific container name (empty = default container)
func (l *LogsClient) GetPodLogs(ctx context.Context, podName string, tailLines *int64, container string) (string, error) {
	opts := &corev1.PodLogOptions{
		TailLines: tailLines,
	}
	if container != "" {
		opts.Container = container
	}

	req := l.clientset.CoreV1().Pods(l.namespace).GetLogs(podName, opts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to stream logs: %w", err)
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", fmt.Errorf("failed to copy logs: %w", err)
	}

	return buf.String(), nil
}

// GetPodLogsByLabel retrieves logs from pods matching a label selector.
// Returns logs from all matching pods concatenated together.
//
// Options:
// - labelSelector: Kubernetes label selector (e.g., "control-plane=controller-manager")
// - tailLines: Number of lines from the end of the logs (nil = all logs)
func (l *LogsClient) GetPodLogsByLabel(ctx context.Context, labelSelector string, tailLines *int64) (string, error) {
	// List pods matching the label selector
	podList, err := l.clientset.CoreV1().Pods(l.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list pods: %w", err)
	}

	if len(podList.Items) == 0 {
		return "", fmt.Errorf("no pods found matching label selector: %s", labelSelector)
	}

	// Get logs from all matching pods
	var allLogs bytes.Buffer
	for i, pod := range podList.Items {
		if i > 0 {
			allLogs.WriteString("\n") // Separate logs from different pods
		}

		logs, err := l.GetPodLogs(ctx, pod.Name, tailLines, "")
		if err != nil {
			// Continue to next pod if one fails
			allLogs.WriteString(fmt.Sprintf("# Failed to get logs from pod %s: %v\n", pod.Name, err))
			continue
		}

		allLogs.WriteString(logs)
	}

	return allLogs.String(), nil
}

// ResourceClient provides a clean interface to fetch Kubernetes resources
// using the native Kubernetes client instead of kubectl commands.
type ResourceClient struct {
	namespace  string
	clientset  *kubernetes.Clientset
	restConfig *rest.Config
}

// NewResourceClient creates a new resource client.
func NewResourceClient(namespace string) (*ResourceClient, error) {
	// Load the kubeconfig from the default location or KUBECONFIG env var
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &ResourceClient{
		namespace:  namespace,
		clientset:  clientset,
		restConfig: config,
	}, nil
}

// GetPodsByLabel retrieves pods matching a label selector.
func (r *ResourceClient) GetPodsByLabel(ctx context.Context, labelSelector string) (*corev1.PodList, error) {
	return r.clientset.CoreV1().Pods(r.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

// GetPod retrieves a specific pod by name.
func (r *ResourceClient) GetPod(ctx context.Context, name string) (*corev1.Pod, error) {
	return r.clientset.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
}

// IsPodReady checks if a pod is in Ready state.
func (r *ResourceClient) IsPodReady(ctx context.Context, podName string) (bool, error) {
	pod, err := r.GetPod(ctx, podName)
	if err != nil {
		return false, err
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue, nil
		}
	}

	return false, nil
}

// CreateNamespace creates a namespace using the Kubernetes API.
func (r *ResourceClient) CreateNamespace(ctx context.Context, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_, err := r.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	return err
}

// DeleteNamespace deletes a namespace using the Kubernetes API.
func (r *ResourceClient) DeleteNamespace(ctx context.Context, name string) error {
	return r.clientset.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
}

// CreateConfigMapFromYAML creates a ConfigMap from YAML data.
func (r *ResourceClient) CreateConfigMapFromYAML(ctx context.Context, name string, data map[string]string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.namespace,
		},
		Data: data,
	}
	_, err := r.clientset.CoreV1().ConfigMaps(r.namespace).Create(ctx, cm, metav1.CreateOptions{})
	return err
}

// DeleteConfigMap deletes a ConfigMap.
func (r *ResourceClient) DeleteConfigMap(ctx context.Context, name string) error {
	return r.clientset.CoreV1().ConfigMaps(r.namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// CreateDeployment creates a Deployment.
func (r *ResourceClient) CreateDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	_, err := r.clientset.AppsV1().Deployments(r.namespace).Create(ctx, deployment, metav1.CreateOptions{})
	return err
}

// DeleteDeployment deletes a Deployment.
func (r *ResourceClient) DeleteDeployment(ctx context.Context, name string) error {
	return r.clientset.AppsV1().Deployments(r.namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// CreateService creates a Service.
func (r *ResourceClient) CreateService(ctx context.Context, service *corev1.Service) error {
	_, err := r.clientset.CoreV1().Services(r.namespace).Create(ctx, service, metav1.CreateOptions{})
	return err
}

// DeleteService deletes a Service.
func (r *ResourceClient) DeleteService(ctx context.Context, name string) error {
	return r.clientset.CoreV1().Services(r.namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// WaitForDeploymentReady waits for a deployment to become ready.
func (r *ResourceClient) WaitForDeploymentReady(ctx context.Context, name string) error {
	// This is a simplified version - in production you'd want to use a Watch
	deployment, err := r.clientset.AppsV1().Deployments(r.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if deployment.Status.ReadyReplicas < *deployment.Spec.Replicas {
		return fmt.Errorf("deployment %s not ready: %d/%d replicas", name, deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
	}

	return nil
}

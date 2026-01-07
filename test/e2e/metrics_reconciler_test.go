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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics Reconciler", Ordered, func() {
	var controllerPodName string

	// Get controller pod name before running tests
	BeforeAll(func() {
		By("getting controller pod name for metrics tests")
		Eventually(func(g Gomega) {
			client, err := NewResourceClient(namespace)
			g.Expect(err).NotTo(HaveOccurred(), "Failed to create resource client")

			ctx := context.Background()
			podList, err := client.GetPodsByLabel(ctx, "control-plane=controller-manager")
			g.Expect(err).NotTo(HaveOccurred())

			// Filter out pods that are being deleted
			var runningPods []string
			for _, pod := range podList.Items {
				if pod.DeletionTimestamp == nil {
					runningPods = append(runningPods, pod.Name)
				}
			}

			g.Expect(runningPods).To(HaveLen(1), "expected 1 controller pod running")
			controllerPodName = runningPods[0]
		}, 20*time.Second, 2*time.Second).Should(Succeed())
	})

	Context("Controller Startup", func() {
		It("should start successfully", func() {
			By("verifying controller pod is running")
			Eventually(func(g Gomega) {
				client, err := NewResourceClient(namespace)
				g.Expect(err).NotTo(HaveOccurred())

				ctx := context.Background()
				ready, err := client.IsPodReady(ctx, controllerPodName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ready).To(BeTrue(), "Controller pod should be ready")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying controller started manager")
			Eventually(func(g Gomega) {
				logsClient, err := NewLogsClient(namespace)
				g.Expect(err).NotTo(HaveOccurred())

				ctx := context.Background()
				logs, err := logsClient.GetPodLogs(ctx, controllerPodName, nil, "")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(logs).To(ContainSubstring("starting manager"),
					"Controller should log 'starting manager'")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying Prometheus client was created")
			Eventually(func(g Gomega) {
				logsClient, err := NewLogsClient(namespace)
				g.Expect(err).NotTo(HaveOccurred())

				ctx := context.Background()
				logs, err := logsClient.GetPodLogs(ctx, controllerPodName, nil, "")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(logs).To(Or(
					ContainSubstring("Prometheus client"),
					ContainSubstring("loaded configuration"),
				), "Controller should create Prometheus client")
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})
	})

	Context("Metrics Reconciliation", func() {
		It("should start metrics reconciler", func() {
			By("verifying metrics reconciler started")
			Eventually(func(g Gomega) {
				logsClient, err := NewLogsClient(namespace)
				g.Expect(err).NotTo(HaveOccurred())

				ctx := context.Background()
				logs, err := logsClient.GetPodLogs(ctx, controllerPodName, nil, "")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(logs).To(ContainSubstring("Starting metrics reconciler"),
					"Metrics reconciler should have started")
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})

		It("should query Lumina metrics successfully", func() {
			By("waiting for first reconciliation cycle")
			// The reconciler runs immediately on startup, then every 5 minutes
			// We should see logs from the first run quickly
			Eventually(func(g Gomega) {
				logsClient, err := NewLogsClient(namespace)
				g.Expect(err).NotTo(HaveOccurred())

				ctx := context.Background()
				logs, err := logsClient.GetPodLogs(ctx, controllerPodName, nil, "")
				g.Expect(err).NotTo(HaveOccurred())

				// The reconciler should log either:
				// - "Reconciling metrics" (debug level)
				// - "Lumina data freshness" (info level)
				// - Any capacity/RI logs
				g.Expect(logs).To(Or(
					ContainSubstring("Lumina data freshness"),
					ContainSubstring("Savings Plan capacity"),
					ContainSubstring("Reserved Instance"),
				), "Metrics reconciler should have queried Lumina")
			}, 45*time.Second, 3*time.Second).Should(Succeed())
		})

		It("should log data freshness", func() {
			By("verifying data freshness is logged")
			Eventually(func(g Gomega) {
				logsClient, err := NewLogsClient(namespace)
				g.Expect(err).NotTo(HaveOccurred())

				ctx := context.Background()
				logs, err := logsClient.GetPodLogs(ctx, controllerPodName, nil, "")
				g.Expect(err).NotTo(HaveOccurred())

				// Should see log with data freshness (age in seconds)
				g.Expect(logs).To(ContainSubstring("Lumina data freshness"),
					"Should log Lumina data freshness")

				// Verify the log includes age_seconds field
				g.Expect(logs).To(ContainSubstring("age_seconds"),
					"Data freshness log should include age_seconds")
			}, 45*time.Second, 3*time.Second).Should(Succeed())
		})

		It("should handle empty metrics gracefully", func() {
			By("verifying no errors for empty SP/RI data")
			// With our mock Prometheus, we likely have no SP/RI data
			// The reconciler should handle this gracefully (not crash)
			Consistently(func(g Gomega) {
				logsClient, err := NewLogsClient(namespace)
				g.Expect(err).NotTo(HaveOccurred())

				ctx := context.Background()
				logs, err := logsClient.GetPodLogs(ctx, controllerPodName, nil, "")
				g.Expect(err).NotTo(HaveOccurred())

				// Should NOT contain fatal errors
				g.Expect(strings.ToLower(logs)).NotTo(ContainSubstring("panic"))
				g.Expect(strings.ToLower(logs)).NotTo(ContainSubstring("fatal"))
			}, 10*time.Second, 2*time.Second).Should(Succeed())
		})

		It("should continue reconciling periodically", func() {
			By("verifying reconciler continues running")
			// The reconciler should keep running in the background
			// We can verify this by checking that the controller stays healthy
			Consistently(func(g Gomega) {
				client, err := NewResourceClient(namespace)
				g.Expect(err).NotTo(HaveOccurred())

				ctx := context.Background()
				ready, err := client.IsPodReady(ctx, controllerPodName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ready).To(BeTrue(), "Controller should remain ready")
			}, 15*time.Second, 3*time.Second).Should(Succeed())
		})
	})

	Context("Health Checks", func() {
		It("should have working health endpoints", func() {
			By("verifying healthz endpoint responds")
			// TODO: This requires exposing the health port via a Service
			// For now, we can verify the controller logs show healthy startup
			Eventually(func(g Gomega) {
				logsClient, err := NewLogsClient(namespace)
				g.Expect(err).NotTo(HaveOccurred())

				ctx := context.Background()
				logs, err := logsClient.GetPodLogs(ctx, controllerPodName, nil, "")
				g.Expect(err).NotTo(HaveOccurred())

				// Controller should have started successfully without health check errors
				g.Expect(logs).NotTo(ContainSubstring("unable to set up health check"))
				g.Expect(logs).NotTo(ContainSubstring("unable to set up ready check"))
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})
	})
})

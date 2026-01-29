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

	"github.com/nextdoor/veneer/pkg/preference"
)

var _ = Describe("NodePool Preference Reconciler", Ordered, func() {
	var controllerPodName string
	var nodePoolClient *NodePoolClient

	// Get controller pod name and create NodePool client before running tests
	BeforeAll(func() {
		By("getting controller pod name for preference tests")
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

		By("creating NodePool client")
		var err error
		nodePoolClient, err = NewNodePoolClient()
		Expect(err).NotTo(HaveOccurred(), "Failed to create NodePool client")
	})

	AfterAll(func() {
		By("cleaning up test NodePools")
		if nodePoolClient != nil {
			ctx := context.Background()
			// Delete any test NodePools
			_ = nodePoolClient.DeleteNodePool(ctx, "test-preferences")
			_ = nodePoolClient.DeleteNodePool(ctx, "test-multi-preferences")
			_ = nodePoolClient.DeleteNodePool(ctx, "test-update-preferences")
		}
	})

	Context("NodePool Reconciler Startup", func() {
		It("should start the NodePool reconciler", func() {
			By("verifying NodePool reconciler configured log")
			Eventually(func(g Gomega) {
				logsClient, err := NewLogsClient(namespace)
				g.Expect(err).NotTo(HaveOccurred())

				ctx := context.Background()
				logs, err := logsClient.GetPodLogs(ctx, controllerPodName, nil, "")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(logs).To(ContainSubstring("NodePool reconciler configured"),
					"NodePool reconciler should have been configured")
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})
	})

	Context("Create Preference Overlays", func() {
		It("should create NodeOverlay from NodePool preferences", func() {
			ctx := context.Background()

			By("creating a NodePool with preference annotations")
			err := nodePoolClient.CreateNodePoolWithPreferences(ctx, "test-preferences", map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a,c7g adjust=-20%",
			})
			Expect(err).NotTo(HaveOccurred(), "Failed to create NodePool with preferences")

			By("waiting for preference overlay to be created")
			Eventually(func(g Gomega) {
				overlays, err := nodePoolClient.ListPreferenceOverlays(ctx)
				g.Expect(err).NotTo(HaveOccurred())

				// Find overlay for our test NodePool
				var found bool
				for _, overlay := range overlays {
					if overlay.Labels[preference.LabelSourceNodePool] == "test-preferences" {
						found = true
						g.Expect(overlay.Name).To(Equal("pref-test-preferences-1"))
						g.Expect(overlay.Labels[preference.LabelManagedBy]).To(Equal(preference.LabelManagedByValue))
						g.Expect(overlay.Labels[preference.LabelPreferenceType]).To(Equal(preference.LabelPreferenceTypeValue))
						g.Expect(*overlay.Spec.PriceAdjustment).To(Equal("-20%"))
						g.Expect(*overlay.Spec.Weight).To(Equal(int32(1)))
					}
				}
				g.Expect(found).To(BeTrue(), "Preference overlay should exist")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying controller logs show overlay creation")
			Eventually(func(g Gomega) {
				logsClient, err := NewLogsClient(namespace)
				g.Expect(err).NotTo(HaveOccurred())

				logs, err := logsClient.GetPodLogs(ctx, controllerPodName, nil, "")
				g.Expect(err).NotTo(HaveOccurred())

				// Should see log about creating the overlay
				g.Expect(logs).To(Or(
					ContainSubstring("Created preference overlay"),
					ContainSubstring("Preference overlay reconciliation complete"),
				), "Should log preference overlay creation")
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})

		It("should create multiple preference overlays", func() {
			ctx := context.Background()

			By("creating a NodePool with multiple preferences")
			err := nodePoolClient.CreateNodePoolWithPreferences(ctx, "test-multi-preferences", map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-10%",
				"veneer.io/preference.5": "kubernetes.io/arch=arm64 adjust=-30%",
				"veneer.io/preference.3": "karpenter.sh/capacity-type=spot adjust=+20%",
			})
			Expect(err).NotTo(HaveOccurred(), "Failed to create NodePool with multiple preferences")

			By("waiting for all preference overlays to be created")
			Eventually(func(g Gomega) {
				overlays, err := nodePoolClient.ListPreferenceOverlays(ctx)
				g.Expect(err).NotTo(HaveOccurred())

				// Count overlays for our test NodePool
				var count int
				for _, overlay := range overlays {
					if overlay.Labels[preference.LabelSourceNodePool] == "test-multi-preferences" {
						count++
					}
				}
				g.Expect(count).To(Equal(3), "Should have 3 preference overlays")
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})
	})

	Context("Update Preference Overlays", func() {
		It("should update overlay when preference changes", func() {
			ctx := context.Background()

			By("creating a NodePool with initial preference")
			err := nodePoolClient.CreateNodePoolWithPreferences(ctx, "test-update-preferences", map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-10%",
			})
			Expect(err).NotTo(HaveOccurred(), "Failed to create NodePool")

			By("waiting for initial overlay to be created")
			Eventually(func(g Gomega) {
				overlays, err := nodePoolClient.ListPreferenceOverlays(ctx)
				g.Expect(err).NotTo(HaveOccurred())

				for _, overlay := range overlays {
					if overlay.Name == "pref-test-update-preferences-1" {
						g.Expect(*overlay.Spec.PriceAdjustment).To(Equal("-10%"))
					}
				}
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("updating the NodePool preference")
			err = nodePoolClient.UpdateNodePoolPreferences(ctx, "test-update-preferences", map[string]string{
				"veneer.io/preference.1": "karpenter.k8s.aws/instance-family=c7a adjust=-25%", // Changed
			})
			Expect(err).NotTo(HaveOccurred(), "Failed to update NodePool")

			By("waiting for overlay to be updated")
			Eventually(func(g Gomega) {
				overlays, err := nodePoolClient.ListPreferenceOverlays(ctx)
				g.Expect(err).NotTo(HaveOccurred())

				for _, overlay := range overlays {
					if overlay.Name == "pref-test-update-preferences-1" {
						g.Expect(*overlay.Spec.PriceAdjustment).To(Equal("-25%"))
					}
				}
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})
	})

	Context("Delete Preference Overlays", func() {
		It("should delete overlay when preference is removed", func() {
			ctx := context.Background()

			By("verifying overlay exists")
			Eventually(func(g Gomega) {
				overlays, err := nodePoolClient.ListPreferenceOverlays(ctx)
				g.Expect(err).NotTo(HaveOccurred())

				var found bool
				for _, overlay := range overlays {
					if overlay.Name == "pref-test-preferences-1" {
						found = true
					}
				}
				g.Expect(found).To(BeTrue())
			}, 10*time.Second, 1*time.Second).Should(Succeed())

			By("removing preference annotation from NodePool")
			err := nodePoolClient.UpdateNodePoolPreferences(ctx, "test-preferences", map[string]string{})
			Expect(err).NotTo(HaveOccurred(), "Failed to remove preferences")

			By("waiting for overlay to be deleted")
			Eventually(func(g Gomega) {
				overlays, err := nodePoolClient.ListPreferenceOverlays(ctx)
				g.Expect(err).NotTo(HaveOccurred())

				// Overlay should no longer exist
				for _, overlay := range overlays {
					g.Expect(overlay.Name).NotTo(Equal("pref-test-preferences-1"),
						"Overlay should have been deleted")
				}
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})

		It("should delete all overlays when NodePool is deleted", func() {
			ctx := context.Background()

			By("verifying overlays exist for test-multi-preferences")
			Eventually(func(g Gomega) {
				overlays, err := nodePoolClient.ListPreferenceOverlays(ctx)
				g.Expect(err).NotTo(HaveOccurred())

				var count int
				for _, overlay := range overlays {
					if overlay.Labels[preference.LabelSourceNodePool] == "test-multi-preferences" {
						count++
					}
				}
				g.Expect(count).To(BeNumerically(">", 0))
			}, 10*time.Second, 1*time.Second).Should(Succeed())

			By("deleting the NodePool")
			err := nodePoolClient.DeleteNodePool(ctx, "test-multi-preferences")
			Expect(err).NotTo(HaveOccurred(), "Failed to delete NodePool")

			By("waiting for all overlays to be cleaned up")
			Eventually(func(g Gomega) {
				overlays, err := nodePoolClient.ListPreferenceOverlays(ctx)
				g.Expect(err).NotTo(HaveOccurred())

				// No overlays should exist for the deleted NodePool
				for _, overlay := range overlays {
					g.Expect(overlay.Labels[preference.LabelSourceNodePool]).NotTo(Equal("test-multi-preferences"),
						"All overlays for deleted NodePool should be cleaned up")
				}
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})
	})

	Context("Error Handling", func() {
		It("should handle invalid preferences gracefully", func() {
			ctx := context.Background()

			By("checking controller doesn't crash with invalid preferences")
			Consistently(func(g Gomega) {
				logsClient, err := NewLogsClient(namespace)
				g.Expect(err).NotTo(HaveOccurred())

				logs, err := logsClient.GetPodLogs(ctx, controllerPodName, nil, "")
				g.Expect(err).NotTo(HaveOccurred())

				// Should NOT contain fatal errors or panics
				g.Expect(strings.ToLower(logs)).NotTo(ContainSubstring("panic"))
				g.Expect(strings.ToLower(logs)).NotTo(ContainSubstring("fatal"))
			}, 10*time.Second, 2*time.Second).Should(Succeed())

			By("verifying controller is still healthy")
			client, err := NewResourceClient(namespace)
			Expect(err).NotTo(HaveOccurred())

			ready, err := client.IsPodReady(ctx, controllerPodName)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue(), "Controller should remain healthy")
		})
	})
})

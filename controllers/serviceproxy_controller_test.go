/*
Copyright 2023.

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

package controllers

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cachev1alpha1 "github.com/thiryn/service-proxy-operator/api/v1alpha1"
)

var _ = Describe("ServiceProxy controller", func() {
	Context("ServiceProxy controller test", func() {
		const ServiceProxyName = "test-service-proxy"

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ServiceProxyName,
				Namespace: ServiceProxyName,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: ServiceProxyName, Namespace: ServiceProxyName}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))

			By("Setting the Image ENV VAR which stores the Operand image")
			err = os.Setenv("SERVICEPROXY_IMAGE", "example.com/image:test")
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)

			By("Removing the Image ENV VAR which stores the Operand image")
			_ = os.Unsetenv("SERVICEPROXY_IMAGE")
		})

		It("should successfully reconcile a custom resource for ServiceProxy", func() {
			By("Creating the custom resource for the Kind ServiceProxy")
			serviceProxy := &cachev1alpha1.ServiceProxy{}
			err := k8sClient.Get(ctx, typeNamespaceName, serviceProxy)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				serviceProxy := &cachev1alpha1.ServiceProxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ServiceProxyName,
						Namespace: namespace.Name,
					},
					Spec: cachev1alpha1.ServiceProxySpec{
						ServiceNames: []string{"some-service"},
					},
				}

				err = k8sClient.Create(ctx, serviceProxy)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &cachev1alpha1.ServiceProxy{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Reconciling the custom resource created")
			// Allow to set the file via env, preempting CLI parameters, not the cleanest, but it'll do the job here
			envNginxConfigFile, ok := os.LookupEnv("NGINX_CONFIG_FILE")
			Expect(ok).To(BeTrue())

			serviceProxyReconciler := &ServiceProxyReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				NginxConfigFile: envNginxConfigFile,
			}

			_, err = serviceProxyReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(Not(HaveOccurred()))

			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func() error {
				found := &appsv1.Deployment{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Checking the latest Status Condition added to the ServiceProxy instance")
			Eventually(func() error {
				if serviceProxy.Status.Conditions != nil && len(serviceProxy.Status.Conditions) != 0 {
					latestStatusCondition := serviceProxy.Status.Conditions[len(serviceProxy.Status.Conditions)-1]
					expectedLatestStatusCondition := metav1.Condition{
						Type:   typeAvailableServiceProxy,
						Status: metav1.ConditionTrue, Reason: "Reconciling",
						Message: fmt.Sprintf("Deployment for custom resource (%s) created successfully", serviceProxy.Name),
					}
					if latestStatusCondition != expectedLatestStatusCondition {
						return fmt.Errorf("The latest status condition added to the serviceProxy instance is not as expected")
					}
				}
				return nil
			}, time.Minute, time.Second).Should(Succeed())
		})
	})
})

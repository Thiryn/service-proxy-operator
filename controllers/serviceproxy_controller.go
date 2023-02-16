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
	"github.com/thiryn/service-proxy-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"os"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
)

// Definitions to manage status conditions
const (
	// typeAvailableServiceProxy represents the status of the Deployment reconciliation
	typeAvailableServiceProxy = "Available"
	// typeDegradedServiceProxy represents the status used when the custom resource is deleted and the finalizer operations are must to occur.
	typeDegradedServiceProxy = "Degraded"
)

// ServiceProxyReconciler reconciles a ServiceProxy object
type ServiceProxyReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	NginxConfigFile string
}

// The following markers are used to generate the rules permissions (RBAC) on config/rbac using controller-gen
// when the command <make manifests> is executed.
// To know more about markers see: https://book.kubebuilder.io/reference/markers.html

//+kubebuilder:rbac:groups=cache.service-proxy-operator.local,resources=serviceproxies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.service-proxy-operator.local,resources=serviceproxies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.service-proxy-operator.local,resources=serviceproxies/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.

// It is essential for the controller's reconciliation loop to be idempotent. By following the Operator
// pattern you will create Controllers which provide a reconcile function
// responsible for synchronizing resources until the desired state is reached on the cluster.
// Breaking this recommendation goes against the design principles of controller-runtime.
// and may lead to unforeseen consequences such as resources becoming stuck and requiring manual intervention.
// For further info:
// - About Operator Pattern: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
// - About Controllers: https://kubernetes.io/docs/concepts/architecture/controller/
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ServiceProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Fetch the ServiceProxy instance
	// The purpose is check if the Custom Resource for the Kind ServiceProxy
	// is applied on the cluster if not we return nil to stop the reconciliation
	ServiceProxy := &v1alpha1.ServiceProxy{}
	err := r.Get(ctx, req.NamespacedName, ServiceProxy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			l.Info("ServiceProxy resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		l.Error(err, "Failed to get ServiceProxy")
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status are available
	if ServiceProxy.Status.Conditions == nil || len(ServiceProxy.Status.Conditions) == 0 {
		meta.SetStatusCondition(&ServiceProxy.Status.Conditions, metav1.Condition{Type: typeAvailableServiceProxy, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, ServiceProxy); err != nil {
			l.Error(err, "Failed to update ServiceProxy status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the ServiceProxy Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, ServiceProxy); err != nil {
			l.Error(err, "Failed to re-fetch ServiceProxy")
			return ctrl.Result{}, err
		}
	}

	if err = r.reconcileConfigMap(ctx, ServiceProxy); err != nil {
		l.Error(err, "Failed to reconcile ConfigMap")
		if updateErr := r.updateStatusConditions(ctx, typeDegradedServiceProxy, "Reconciling",
			fmt.Sprintf("Failed to reconcile the Nginx ConfigMap for (%s): (%s)", ServiceProxy.Name, err), ServiceProxy); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}
	// Set the services name in the status.
	ServiceProxy.Status.ServiceNames = ServiceProxy.Spec.ServiceNames
	if updateErr := r.Status().Update(ctx, ServiceProxy); err != nil {
		log.FromContext(ctx).Error(err, "Failed to update ServiceProxy status")
		return ctrl.Result{}, updateErr
	}

	if err = r.createDeployment(ctx, ServiceProxy); err != nil {
		l.Error(err, "Failed to reconcile Deployment")
		if updateErr := r.updateStatusConditions(ctx, typeDegradedServiceProxy, "Reconciling",
			fmt.Sprintf("Failed to reconcile the Nginx Deployment for (%s): (%s)", ServiceProxy.Name, err), ServiceProxy); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}
	if err = r.createService(ctx, ServiceProxy); err != nil {
		log.FromContext(ctx).Error(err, "Failed to reconcile Service")
		if updateErr := r.updateStatusConditions(ctx, typeDegradedServiceProxy, "Reconciling",
			fmt.Sprintf("Failed to reconcile the Service for (%s): (%s)", ServiceProxy.Name, err), ServiceProxy); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}
	if err = r.createIngress(ctx, ServiceProxy); err != nil {
		log.FromContext(ctx).Error(err, "Failed to reconcile Ingress")
		if updateErr := r.updateStatusConditions(ctx, typeDegradedServiceProxy, "Reconciling",
			fmt.Sprintf("Failed to reconcile the Ingress for (%s): (%s)", ServiceProxy.Name, err), ServiceProxy); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}
	if updateErr := r.updateStatusConditions(ctx, typeAvailableServiceProxy, "Ready",
		fmt.Sprintf("Service Proxy is ready: %s", ServiceProxy.Name), ServiceProxy); updateErr != nil {
		return ctrl.Result{}, updateErr
	}
	return ctrl.Result{}, nil
}

func (r *ServiceProxyReconciler) updateStatusConditions(ctx context.Context, conditionType, reason, message string, ServiceProxy *v1alpha1.ServiceProxy) error {
	condition := metav1.Condition{Type: conditionType,
		Status: metav1.ConditionTrue, Reason: reason,
		Message: message}
	if reason != typeAvailableServiceProxy {
		condition.Status = metav1.ConditionFalse
	}

	// Don't update the status if the conditions are the same
	if !reflect.DeepEqual(condition, ServiceProxy.Status.Conditions) {
		meta.SetStatusCondition(&ServiceProxy.Status.Conditions, condition)
		if err := r.Status().Update(ctx, ServiceProxy); err != nil {
			log.FromContext(ctx).Error(err, "Failed to update ServiceProxy status")
			return err
		}
	} else {
		log.FromContext(ctx).Info("Status not changed")
	}
	return nil
}

// labelsForServiceProxy returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForServiceProxy(name string) map[string]string {
	var imageTag string
	image, err := imageForServiceProxy()
	if err == nil {
		imageTag = strings.Split(image, ":")[1]
	}
	return map[string]string{"app.kubernetes.io/name": "ServiceProxy",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/part-of":    "service-proxy-operator",
		"app.kubernetes.io/created-by": "controller-manager",
	}
}

// imageForServiceProxy gets the Operand image which is managed by this controller
// from the SERVICEPROXY_IMAGE environment variable defined in the config/manager/manager.yaml
func imageForServiceProxy() (string, error) {
	var imageEnvVar = "SERVICEPROXY_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return "", fmt.Errorf("Unable to find %s environment variable with the image", imageEnvVar)
	}
	return image, nil
}

// SetupWithManager sets up the controller with the Manager.
// Note that the Deployment will be also watched in order to ensure its
// desirable state on the cluster
func (r *ServiceProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ServiceProxy{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

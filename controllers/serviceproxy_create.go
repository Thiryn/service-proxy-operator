package controllers

import (
	"context"
	"os"
	"strings"

	"github.com/thiryn/service-proxy-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *ServiceProxyReconciler) reconcileConfigMap(ctx context.Context, serviceProxy *v1alpha1.ServiceProxy) error {
	l := log.FromContext(ctx)

	var currentConfigmap corev1.ConfigMap
	if err := r.Get(ctx,
		client.ObjectKey{
			Name:      serviceProxy.GetConfigMapName(),
			Namespace: serviceProxy.Namespace,
		}, &currentConfigmap); err != nil {
		if apierrors.IsNotFound(err) {
			// The configmap does not exist, create it
			l.Info("ConfigMap not found, creating")
			if newConfigMap, err := r.makeConfigMap(serviceProxy); err != nil {
				return err
			} else {
				if err = ctrl.SetControllerReference(serviceProxy, newConfigMap, r.Scheme); err != nil {
					return err
				}
				log.FromContext(ctx).Info("Creating a new ConfigMap",
					"ConfigMap.Namespace", newConfigMap.Namespace, "ConfigMap.Name", newConfigMap.Name)

				return r.Create(ctx, newConfigMap)
			}
		}
		return err
	}
	l.Info("ConfigMap found, check for changes")
	// The configmap has been found. Let's check if we need to update it
	if !serviceProxy.ServiceListMatches(serviceProxy.Status.ServiceNames) {
		l.Info("Configmap changes found, updating")
		// services are not matching, update the config
		if newConfigMap, err := r.makeConfigMap(serviceProxy); err != nil {
			return err
		} else {
			l.Info("Updating ConfigMap",
				"ConfigMap.Namespace", newConfigMap.Namespace,
				"ConfigMap.Name", newConfigMap.Name,
				"ConfigMap.OldService", serviceProxy.Status.ServiceNames,
				"ConfigMap.NewService", serviceProxy.Spec.ServiceNames)
			return r.Update(ctx, newConfigMap)
		}
	}
	// The config is up to date, nothing to do
	return nil
}

func (r *ServiceProxyReconciler) createIngress(ctx context.Context, serviceProxy *v1alpha1.ServiceProxy) error {
	var currentIngress networkingv1.Ingress
	if err := r.Get(ctx,
		client.ObjectKey{
			Name:      serviceProxy.GetIngressName(),
			Namespace: serviceProxy.Namespace,
		}, &currentIngress); err != nil {
		if apierrors.IsNotFound(err) {
			labels := labelsForServiceProxy(serviceProxy.Name)
			name := serviceProxy.GetIngressName()
			ingressPathType := networkingv1.PathTypePrefix
			newIngress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: serviceProxy.Namespace,
					Labels:    labels,
					Annotations: map[string]string{
						"nginx.ingress.kubernetes.io/rewrite-target": "/$1",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{{
						Host: serviceProxy.Spec.IngressDomainMatch,
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										Path:     "/(.*)",
										PathType: &ingressPathType,
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: serviceProxy.GetServiceName(),
												Port: networkingv1.ServiceBackendPort{
													Name: v1alpha1.ServiceProxyHTTPPortName,
												},
											},
										},
									},
								},
							},
						},
					}},
				},
			}
			// Set the ownerRef
			if err = ctrl.SetControllerReference(serviceProxy, newIngress, r.Scheme); err != nil {
				return err
			}

			log.FromContext(ctx).Info("Creating a new Ingress",
				"Ingress.Namespace", newIngress.Namespace, "Ingress.Name", newIngress.Name)
			return r.Create(ctx, newIngress)
		}
		return err
	}
	// Ingress has been found, we don't have any change to check against at the moment
	return nil
}

func (r *ServiceProxyReconciler) createService(ctx context.Context, serviceProxy *v1alpha1.ServiceProxy) error {
	var currentService corev1.Service
	if err := r.Get(ctx,
		client.ObjectKey{
			Name:      serviceProxy.GetServiceName(),
			Namespace: serviceProxy.Namespace,
		}, &currentService); err != nil {
		if apierrors.IsNotFound(err) {
			labels := labelsForServiceProxy(serviceProxy.Name)

			newService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceProxy.GetServiceName(),
					Namespace: serviceProxy.Namespace,
					Labels:    labels,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name: v1alpha1.ServiceProxyHTTPPortName,
						Port: 8080,
						TargetPort: intstr.IntOrString{
							Type:   intstr.String,
							StrVal: v1alpha1.ServiceProxyHTTPPortName,
						},
					}},
					Selector: labels,
				},
			}
			// Set the ownerRef
			if err = ctrl.SetControllerReference(serviceProxy, newService, r.Scheme); err != nil {
				return err
			}

			log.FromContext(ctx).Info("Creating a new Service",
				"Service.Namespace", newService.Namespace, "Service.Name", newService.Name)
			return r.Create(ctx, newService)
		}
		return err
	}
	// Service has been found, we don't have any change to check against at the moment
	return nil
}

func (r *ServiceProxyReconciler) createDeployment(ctx context.Context, serviceProxy *v1alpha1.ServiceProxy) error {
	l := log.FromContext(ctx)

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: serviceProxy.Name, Namespace: serviceProxy.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		dep, err := r.makeDeployment(serviceProxy)
		if err != nil {
			l.Error(err, "Failed to define new Deployment resource for ServiceProxy")
			return err
		}

		l.Info("Creating a new Deployment",
			"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		if err = r.Create(ctx, dep); err != nil {
			l.Error(err, "Failed to create new Deployment",
				"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return err
		}
		// Deployment created successfully
		return nil
	} else if err != nil {
		l.Error(err, "Failed to get Deployment")
		// Let's return the error for the reconciliation be re-trigged again
		return err
	}
	return nil
}

func (r *ServiceProxyReconciler) makeConfigMap(serviceProxy *v1alpha1.ServiceProxy) (*corev1.ConfigMap, error) {
	// We're using the serviceNames to match on specific services to proxy for. This is injected in the nginx config
	// through regexp. The service names are constrained by the kubernetes API syntax, it should not be possible
	// to mess up with the regexp here
	serviceString := strings.Join(serviceProxy.Spec.ServiceNames, "|")
	configTemplate, err := os.ReadFile(r.NginxConfigFile)
	if err != nil {
		return nil, err
	}
	nginxConfig := strings.Replace(string(configTemplate), "{SERVICE_MATCH}", serviceString, 1)
	nginxConfig = strings.Replace(nginxConfig, "{NAMESPACE}", serviceProxy.Namespace, 1)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceProxy.GetConfigMapName(),
			Namespace: serviceProxy.Namespace,
			Labels:    labelsForServiceProxy(serviceProxy.Name),
		},
		Data: map[string]string{
			v1alpha1.ConfigMapKey: nginxConfig,
		},
	}
	// Set the ownerRef
	if err := ctrl.SetControllerReference(serviceProxy, configMap, r.Scheme); err != nil {
		return nil, err
	}
	return configMap, nil
}

// makeDeployment returns a ServiceProxy Deployment object
func (r *ServiceProxyReconciler) makeDeployment(
	serviceProxy *v1alpha1.ServiceProxy,
) (*appsv1.Deployment, error) {
	labels := labelsForServiceProxy(serviceProxy.Name)

	// Get the Operand image
	image, err := imageForServiceProxy()
	if err != nil {
		return nil, err
	}

	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceProxy.Name,
			Namespace: serviceProxy.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						// IMPORTANT: seccomProfile was introduced with Kubernetes 1.19
						// If you are looking for to produce solutions to be supported
						// on lower versions you must remove this option.
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Image:           image,
						Name:            "nginx",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Ports: []corev1.ContainerPort{{
							Name:          v1alpha1.ServiceProxyHTTPPortName,
							ContainerPort: 8080, // Same port as in the nginx config
						}},
						// Ensure restrictive context for the container
						// More info: https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             &[]bool{true}[0],
							RunAsUser:                &[]int64{1001}[0],
							AllowPrivilegeEscalation: &[]bool{false}[0],
							Capabilities: &corev1.Capabilities{
								Add: []corev1.Capability{
									"NET_BIND_SERVICE",
								},
								Drop: []corev1.Capability{
									"ALL",
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "nginx-config",
							ReadOnly:  true,
							MountPath: "/etc/nginx/nginx.conf",
							SubPath:   "nginx.conf",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "nginx-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: serviceProxy.GetConfigMapName(),
								},
							},
						},
					}},
				},
			},
		},
	}

	// Set the ownerRef
	if err := ctrl.SetControllerReference(serviceProxy, dep, r.Scheme); err != nil {
		return nil, err
	}
	return dep, nil
}

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

package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	ConfigMapKey             = "nginx.conf"
	ServiceProxyHTTPPortName = "http"
)

// ServiceProxySpec defines the desired state of ServiceProxy
type ServiceProxySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ServiceNames points to the services the proxy should be in effect for
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	ServiceNames []string `json:"serviceNames"`

	// ServiceNames points to the services the proxy should be in effect for
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// +optional
	// +kubebuilder:default:=""
	IngressDomainMatch string `json:"ingressDomainMatch"`
}

// ServiceProxyStatus defines the observed state of ServiceProxy
type ServiceProxyStatus struct {
	// ServiceNames represents the names of services currently being proxied
	// +operator-sdk:csv:customresourcedefinitions:type=status
	// +optional
	ServiceNames []string `json:"serviceNames"`

	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ServiceProxy is the Schema for the serviceproxies API
type ServiceProxy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceProxySpec   `json:"spec,omitempty"`
	Status ServiceProxyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ServiceProxyList contains a list of ServiceProxy
type ServiceProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceProxy `json:"items"`
}

func (s ServiceProxy) GetConfigMapName() string {
	return fmt.Sprintf("%s-nginx-conf", s.Name)
}

func (s ServiceProxy) GetServiceName() string {
	return fmt.Sprintf("%s-service", s.Name)
}

func (s ServiceProxy) GetIngressName() string {
	return fmt.Sprintf("%s-ingress", s.Name)
}

// ServiceListMatches returns true if the serviceList in parameter matches the one in the spec, false if not
func (s ServiceProxy) ServiceListMatches(serviceList []string) bool {
	if len(serviceList) != len(s.Spec.ServiceNames) {
		// different size list means they can't be the same
		return false
	}
	// Same size, let's do a naive O(n*n) loop search
	for _, requiredService := range serviceList {
		found := false
		for _, currentService := range s.Spec.ServiceNames {
			if currentService == requiredService {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func init() {
	SchemeBuilder.Register(&ServiceProxy{}, &ServiceProxyList{})
}

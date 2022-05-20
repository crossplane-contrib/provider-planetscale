/*
Copyright 2022 The Crossplane Authors.

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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// PasswordParameters are the configurable fields of a Password.
type PasswordParameters struct {
	ConfigurableField string `json:"configurableField"`
}

// PasswordObservation are the observable fields of a Password.
type PasswordObservation struct {
	ObservableField string `json:"observableField,omitempty"`
}

// A PasswordSpec defines the desired state of a Password.
type PasswordSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       PasswordParameters `json:"forProvider"`
}

// A PasswordStatus represents the observed state of a Password.
type PasswordStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          PasswordObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A Password is an example API type.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,planetscale}
type Password struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PasswordSpec   `json:"spec"`
	Status PasswordStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PasswordList contains a list of Password
type PasswordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Password `json:"items"`
}

// Password type metadata.
var (
	PasswordKind             = reflect.TypeOf(Password{}).Name()
	PasswordGroupKind        = schema.GroupKind{Group: Group, Kind: PasswordKind}.String()
	PasswordKindAPIVersion   = PasswordKind + "." + SchemeGroupVersion.String()
	PasswordGroupVersionKind = SchemeGroupVersion.WithKind(PasswordKind)
)

func init() {
	SchemeBuilder.Register(&Password{}, &PasswordList{})
}

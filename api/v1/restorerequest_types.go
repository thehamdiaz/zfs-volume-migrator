/*
Copyright 2023 thehamdiaz.

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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RestoreRequestSpec defines the desired state of RestoreRequest
type RestoreRequestSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of RestoreRequest. Edit restorerequest_types.go to remove/update
	Names      Names
	Parameters Parameters
}

type Names struct {
	MigrationRequestName string `json:"migrationRequestName"`
	StorageClassName     string `json:"storageClassName"`
	PVName               string `json:"pvName"`
	PVCName              string `json:"pvcName"`
	ZFSDatasetName       string `json:"zfsDatasetName"`
	ZFSPoolName          string `json:"zfsPoolName"`
	TargetNodeName       string `json:"targetNodeName"`
}

type Parameters struct {
	Capacity      resource.Quantity                    `json:"capacity"`
	AccessModes   []corev1.PersistentVolumeAccessMode  `json:"accessModes"`
	ReclaimPolicy corev1.PersistentVolumeReclaimPolicy `json:"reclaimPolicy"`
	PVCResources  corev1.ResourceRequirements          `json:"pvcResources"`
}

// RestoreRequestStatus defines the observed state of RestoreRequest
type RestoreRequestStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ReceivedSnapshots int64  `json:"receivedSnapshots,omitempty"`
	DatasetReady      string `json:"datasetReady,omitempty"`
	Succeeded         string `json:"succeeded,omitempty"`
	Message           string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// RestoreRequest is the Schema for the restorerequests API
type RestoreRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestoreRequestSpec   `json:"spec,omitempty"`
	Status RestoreRequestStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RestoreRequestList contains a list of RestoreRequest
type RestoreRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RestoreRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RestoreRequest{}, &RestoreRequestList{})
}

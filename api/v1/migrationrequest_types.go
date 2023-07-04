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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MigrationRequestSpec defines the desired state of MigrationRequest
type MigrationRequestSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of MigrationRequest. Edit migrationrequest_types.go to remove/update

	PodName     string `json:"podName,omitempty"`
	Destination DestinationDef
}

type DestinationDef struct {
	User          string `json:"user,omitempty"`
	RemotePool    string `json:"remotePool,omitempty"`
	RemoteDataset string `json:"remoteDataset,omitempty"`
	RemoteHost    string `json:"remoteHost,omitempty"`
}

// MigrationRequestStatus defines the observed state of MigrationRequest
type MigrationRequestStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	SnapshotCount       string `json:"snapshotCreated,omitempty"`
	AllSnapshotsCreated string `json:"allSnapshotCreated,omitempty"`
	MigrationComplete   string `json:"migrationComplete,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MigrationRequest is the Schema for the migrationrequests API
type MigrationRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MigrationRequestSpec   `json:"spec,omitempty"`
	Status MigrationRequestStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MigrationRequestList contains a list of MigrationRequest
type MigrationRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MigrationRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MigrationRequest{}, &MigrationRequestList{})
}

/*
Copyright 2024.

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
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ModelSpec defines the desired state of Model
type ModelSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// SerialNumber is the unique serial number of the entity's hardware.
	ModelNumber string `json:"serialNumber,omitempty"`

	// Description provides a human-readable description of the Model.
	Description string `json:"description,omitempty"`

	// DockerImage is a string field representing the container image to use for the Model.
	DockerImage string `json:"dockerImage,omitempty"`

	// IngressURl
	IngressUrl string `json:"ingressUrl,omitempty"`

	// Statefulset provided by customer, the operator will setup ownership. It will take priority
	StatefulSetName string `json:"statefulSetName,omitempty"`

	// Latest batch replicas of entity within the model
	TargetReplicas *int32 `json:"targetReplicas,omitempty"`

	// Self defined endpoint for operator sending task to
	TaskEndpoint string `json:"taskEndpoint,omitempty"`

	// self defined endpoint for control pannel
	CtrlEndpoint string `json:"ctrlEndpoint,omitempty"`

	// Self defined endpoint for operator to query status from
	StatusEndpoint string `json:"statusEndpoint,omitempty"`

	// SyncPeriod specifies how often the entity should sync its status
	// with the associated Pod, expressed in seconds.
	// A value of 0 or less indicates no periodic syncing.
	DefaultSyncPeriod int32 `json:"syncPeriod,omitempty"`

	// Wheather to have active/standby mode
	EnableSecondary bool `json:"enableSecondary,omitempty"`
}

// ModelStatus defines the observed state of Model
type ModelStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ActiveEntities represents the number of entities associated with this model that are currently active.
	DeactiveEntities []string `json:"deactiveEntities,omitempty"`

	// DisconnectedEntities represents the number of entities associated with this model that are currently disconnected.
	DisconnectedEntities []string `json:"disconnectedEntities,omitempty"`

	// Conditions represents the latest available observations of an Entity's current state.
	Conditions []Condition `json:"conditions,omitempty"`

	// Phase show the current object status
	Phase string `json:"phase,omitempty"`

	//
	CurrentReplicas string `json:"currentReplicas,omitempty"`

	//
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:webhook:path=/validate-robokube-meliazeng-github-io-v1-model,mutating=false,failurePolicy=fail,sideEffects=None,groups=robokube.meliazeng.github.io,resources=models,verbs=create;update,versions=v1,name=vmodel.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/mutate-robokube-meliazeng-github-io-v1-model,mutating=true,failurePolicy=fail,sideEffects=None,groups=robokube.meliazeng.github.io,resources=models,verbs=create;update,versions=v1,name=mmodel.kb.io,admissionReviewVersions=v1
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Replicas",type=string,JSONPath=`.status.currentReplicas`

// Model is the Schema for the models API
type Model struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelSpec   `json:"spec,omitempty"`
	Status ModelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelList contains a list of Model
type ModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Model `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Model{}, &ModelList{})
}

// Example implementation of GetObjectsToDelete for Model type
func (m *Model) GetObjectsToDelete() ([]func(ctx context.Context, r client.Client) error, error) {
	// Logic to fetch objects associated with Model (e.g., StatefulSet)
	return []func(ctx context.Context, r client.Client) error{
		// ... add more deletion functions for other associated objects
	}, nil
}

// GetConditions implements the Conditioned interface for Entity.
func (e *Model) GetConditions() []Condition {
	return e.Status.Conditions
}

// SetConditions implements the Conditioned interface for Entity.
func (e *Model) SetConditions(conditions []Condition) {
	e.Status.Conditions = conditions
}

// SetConditions implements the Conditioned interface for Entity.
func (e *Model) GetPhase() string {
	return e.Status.Phase
}

// SetConditions implements the Conditioned interface for Entity.
func (e *Model) SetPhase(phase string) {
	e.Status.Phase = phase
	if phase == StatusProvisioned {
		e.Status.ObservedGeneration = e.GetGeneration()
	}
}

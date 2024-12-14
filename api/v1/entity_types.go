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

/*
Todo:
1. mutate the pod to add certificate
2. admission control on pod provison based on offboard
*/

package v1

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EntitySpec defines the desired state of Entity
type EntitySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Identity represents a unique identifier for the entity.
	Identity string `json:"identity,omitempty"`

	// Organization denotes the organization or group the entity belongs to.
	Organization string `json:"organization,omitempty"`

	// Privileged indicates whether the entity has elevated privileges.
	Privileged bool `json:"privileged,omitempty"`

	// OnSecondary indicated whether the backend pod is from primary or secondary statefulset
	OnSecondary bool `json:"onSecondary,omitempty"`

	// SerialNumber is the unique serial number of the entity's hardware.
	SerialNumber string `json:"serialNumber,omitempty"`

	// it will be the must field, otherwise will be blocked
	OrdinalIndex *int32 `json:"ordinalIndex,omitempty"`

	// Model is a reference to the Model associated with this entity. this will be set by ownerReference
	Model string `json:"model"`

	// Secret that will be used by the entity
	Secret string `json:"secret,omitempty"`

	// SyncPeriod specifies how often the entity should sync its status
	// with the associated Pod, expressed in seconds.
	// A value of 0 or less indicates no periodic syncing.
	SyncPeriod int32 `json:"syncPeriod,omitempty"`

	// Usage is a string field representing how the entity is being used.
	Usage string `json:"usage,omitempty"`

	// is decommissioned
	Decommissioned bool `json:"decommissioned,omitempty"`
}

// EntityStatus defines the observed state of Entity
type EntityStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Url to access the entity control pannel, task and status endpoint
	Urls map[string]string `json:"urls,omitempty"`

	// pod name
	PodName string `json:"podName,omitempty"`

	// Conditions represents the latest available observations of an Entity's current state.
	Conditions []Condition `json:"conditions,omitempty"`

	// Data holds arbitrary JSON data associated with the entity.
	Data apiextensions.JSON `json:"data,omitempty"`

	// LastSyncTime represents the last time the entity's status was synced with the associated Pod.
	LastSyncTime metav1.Time `json:"lastSyncTime,omitempty"`

	// Phase show the current object status
	Phase string `json:"phase,omitempty"`

	//
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:webhook:path=/validate-robokube-meliazeng-github-io-v1-entity,mutating=false,failurePolicy=fail,sideEffects=None,groups=robokube.meliazeng.github.io,resources=entities,verbs=create;update,versions=v1,name=ventity.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/mutate-robokube-meliazeng-github-io-v1-entity,mutating=true,failurePolicy=fail,sideEffects=None,groups=robokube.meliazeng.github.io,resources=entities,verbs=create;update,versions=v1,name=mentity.kb.io,admissionReviewVersions=v1
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Pod",type=string,JSONPath=`.status.podName`
// +kubebuilder:printcolumn:name="Identity",type=string,JSONPath=`.spec.identity`

// Entity is the Schema for the entities API
type Entity struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EntitySpec   `json:"spec,omitempty"`
	Status EntityStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EntityList contains a list of Entity
type EntityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Entity `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Entity{}, &EntityList{})
}

// GetConditions implements the Conditioned interface for Entity.
func (e *Entity) GetConditions() []Condition {
	return e.Status.Conditions
}

// SetConditions implements the Conditioned interface for Entity.
func (e *Entity) SetConditions(conditions []Condition) {
	e.Status.Conditions = conditions
}

// SetConditions implements the Conditioned interface for Entity.
func (e *Entity) GetPhase() string {
	return e.Status.Phase
}

// SetConditions implements the Conditioned interface for Entity.
func (e *Entity) SetPhase(phase string) {
	e.Status.Phase = phase
	if phase == StatusProvisioned {
		e.Status.ObservedGeneration = e.GetGeneration()
	}
}

func (e *Entity) GetObjectsToDelete() ([]func(ctx context.Context, r client.Client) error, error) {
	// Logic to fetch objects associated with Model (e.g., StatefulSet)
	return []func(ctx context.Context, r client.Client) error{
		// delete the pod
		func(ctx context.Context, r client.Client) error {
			pod := &corev1.Pod{}
			pod.Name = e.Status.PodName
			pod.Namespace = e.Namespace
			return r.Delete(ctx, pod)
		},
	}, nil
}

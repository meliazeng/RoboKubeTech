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
	"fmt"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GroupSpec defines the desired state of Group
type GroupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Description provides a human-readable description of the Group.
	Description string `json:"description,omitempty"`

	// Owner specifies the owner or maintainer of the Group.
	Owner string `json:"owner,omitempty"`

	// Contact provides contact information for the Group.
	Contact string `json:"contact,omitempty"`

	// Last applied targets
	LastTargets *[]TargetRef `json:"lastTargets,omitempty"`

	// Type is the category of the group. Management group will allow smart pod to be able to manage the entity within the group
	Type string `json:"type"`
}

// GroupStatus defines the observed state of Group
type GroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// TotalEntities contain the number of entities that belong to this group.
	TotalEntities int `json:"totalEntities,omitempty"`

	// Conditions represents the latest available observations of an Entity's current state.
	Conditions []Condition `json:"conditions,omitempty"`

	//
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase show the current object status
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:webhook:path=/validate-robokube-meliazeng-github-io-v1-group,mutating=false,failurePolicy=fail,sideEffects=None,groups=robokube.meliazeng.github.io,resources=groups,verbs=create;update,versions=v1,name=vgroup.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/mutate-robokube-meliazeng-github-io-v1-group,mutating=true,failurePolicy=fail,sideEffects=None,groups=robokube.meliazeng.github.io,resources=groups,verbs=create;update,versions=v1,name=mgroup.kb.io,admissionReviewVersions=v1
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Group is the Schema for the groups API
type Group struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GroupSpec   `json:"spec,omitempty"`
	Status GroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GroupList contains a list of Group
type GroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Group `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Group{}, &GroupList{})
}

// GetConditions implements the Conditioned interface for Entity.
func (e *Group) GetConditions() []Condition {
	return e.Status.Conditions
}

// SetConditions implements the Conditioned interface for Entity.
func (e *Group) SetConditions(conditions []Condition) {
	e.Status.Conditions = conditions
}

// SetConditions implements the Conditioned interface for Entity.
func (e *Group) GetPhase() string {
	return e.Status.Phase
}

// SetConditions implements the Conditioned interface for Entity.
func (e *Group) SetPhase(phase string) {
	e.Status.Phase = phase
	if phase == StatusProvisioned {
		e.Status.ObservedGeneration = e.GetGeneration()
	}
}

// Example implementation of GetObjectsToDelete for Model type
func (e *Group) GetObjectsToDelete() ([]func(ctx context.Context, r client.Client) error, error) {
	return []func(ctx context.Context, r client.Client) error{
		// remove netpol
		func(ctx context.Context, r client.Client) error {
			np := &networkingv1.NetworkPolicy{}
			npNamespacedName := types.NamespacedName{
				Namespace: e.Namespace,
				Name:      e.Name + "-netpol",
			}
			if err := r.Get(ctx, npNamespacedName, np); err != nil {
				// Handle error, maybe entity doesn't exist
				return fmt.Errorf("failed to get entity: %w", err)
			}
			return r.Delete(ctx, np)
		},
		// remove label for entities
		func(ctx context.Context, r client.Client) error {
			entityList := &EntityList{}
			labelSelector, err := labels.Parse(RoboKubeGroupKey + e.Name)
			if err != nil {
				return fmt.Errorf("failed to create label selector")
			}
			if err := r.List(ctx, entityList, &client.ListOptions{
				Namespace:     e.Namespace,
				LabelSelector: labelSelector,
			}); err != nil {
				return fmt.Errorf("failed to list entities: %w", err)
			}

			labelModificationFunc := BuildLabelObj(RoboKubeGroupKey+e.Name, "", ResourceLabelDelete)
			for i := range entityList.Items {
				labelModificationFunc(&entityList.Items[i])
				if err := r.Update(ctx, &entityList.Items[i]); err != nil {
					return fmt.Errorf("failed to update entity: %w", err)
				}
			}
			return nil
		},
		// remove label for pods
		func(ctx context.Context, r client.Client) error {
			podList := &corev1.PodList{}
			labelSelector, err := labels.Parse(RoboKubeGroupKey + e.Name)
			if err != nil {
				return fmt.Errorf("failed to create label selector")
			}
			if err := r.List(ctx, podList, &client.ListOptions{
				Namespace:     e.Namespace,
				LabelSelector: labelSelector,
			}); err != nil {
				return fmt.Errorf("failed to list entities: %w", err)
			}

			labelModificationFunc := BuildLabelObj(RoboKubeGroupKey+e.Name, "", ResourceLabelDelete)
			for i := range podList.Items {
				labelModificationFunc(&podList.Items[i])
				if err := r.Update(ctx, &podList.Items[i]); err != nil {
					return fmt.Errorf("failed to update entity: %w", err)
				}
			}
			return nil
		},
	}, nil
}

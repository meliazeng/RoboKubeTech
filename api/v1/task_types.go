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
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TaskSpec defines the desired state of Task
type TaskSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Description provides a human-readable description of the Task.
	Description string `json:"description,omitempty"`

	// DockerImage is a string field representing the container image to use for the Task.
	DockerImage string `json:"dockerImage,omitempty"`

	// Schedule is a cron expression string that defines the schedule for running the Task.
	Schedule string `json:"schedule,omitempty"`

	// The time when the task will be scheduled to run once. if Schedule is set, this field will be when the schedule ends
	RunTime *metav1.Time `json:"runTime,omitempty"`

	// offset period where the task will be executed before or cut off after
	Offset int32 `json:"offset,omitempty"`

	// Payload is a JSON object that contains data to be passed to the task in github action format
	Payload apiextensions.JSON `json:"payload"`

	// Task will be assign to
	Targets *[]TargetRef `json:"targets,omitempty"`

	// Run the task without actually call the entity to test if payload is valid
	DryRun bool `json:"dryRun,omitempty"`

	// UnConditonalRun means the task will be run regardless of the entity's phase
	UnConditonalRun bool `json:"unConditonalRun,omitempty"`

	// Persistant meaning the task will be remain after completion
	Persistant bool `json:"persistant,omitempty"`
}

type TargetType string

const (
	// TargetTypeEntity represents an entity target.
	TargetTypeEntity TargetType = "Entity"
	// TargetTypeGroup represents a group target.
	TargetTypeGroup TargetType = "Group"
	// TargetTypeModel represents a model target.
	TargetTypeModel TargetType = "Model"
)

// TargetRef specifies the target of a task.
type TargetRef struct {
	// Type is the type of the target.
	Type TargetType `json:"type"`
	// Name is the name of the target.
	Name string `json:"name"`
	// Action
	Action ResourceAction `json:"action,omitempty"`
}

// TaskStatus defines the observed state of Task
type TaskStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions represents the latest available observations of a Task's current state.
	Conditions []Condition `json:"conditions,omitempty"`

	//
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase indicates the current phase of the Task (e.g., Pending, Running, Completed, Failed).
	Phase string `json:"phase,omitempty"`

	//
	FailedEntities []string `json:"failedEntities,omitempty"`

	// ExecuteTime is a timestamp indicating when the Task should be executed.
	ExecuteTime metav1.Time `json:"executeTime,omitempty"`

	// CompletedTime is a timestamp indicating when the Task was completed.
	CompletedTime metav1.Time `json:"completedTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:webhook:path=/validate-robokube-meliazeng-github-io-v1-task,mutating=false,failurePolicy=fail,sideEffects=None,groups=robokube.meliazeng.github.io,resources=tasks,verbs=create;update,versions=v1,name=vtask.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/mutate-robokube-meliazeng-github-io-v1-task,mutating=true,failurePolicy=fail,sideEffects=None,groups=robokube.meliazeng.github.io,resources=tasks,verbs=create;update,versions=v1,name=mtask.kb.io,admissionReviewVersions=v1
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Task is the Schema for the tasks API
type Task struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TaskSpec   `json:"spec,omitempty"`
	Status TaskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TaskList contains a list of Task
type TaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Task `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Task{}, &TaskList{})
}

// GetConditions implements the Conditioned interface for Entity.
func (e *Task) GetConditions() []Condition {
	return e.Status.Conditions
}

// SetConditions implements the Conditioned interface for Entity.
func (e *Task) SetConditions(conditions []Condition) {
	e.Status.Conditions = conditions
}

// SetConditions implements the Conditioned interface for Entity.
func (e *Task) GetPhase() string {
	return e.Status.Phase
}

// SetConditions implements the Conditioned interface for Entity.
func (e *Task) SetPhase(phase string) {
	e.Status.Phase = phase
	if phase == StatusProvisioned {
		e.Status.ObservedGeneration = e.GetGeneration()
	}
}

func (t *Task) GetObjectsToDelete() ([]func(ctx context.Context, r client.Client) error, error) {
	// Logic to fetch objects associated with Model (e.g., StatefulSet)
	return []func(ctx context.Context, r client.Client) error{}, nil
}

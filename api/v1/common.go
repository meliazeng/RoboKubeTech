package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Define allowed backend styles
type BackendStyle string

const (
	BackendStyleStatefulSet BackendStyle = "statefulset"
	BackendStyleDeployment  BackendStyle = "deployment"
	BackendStyleReplicaSet  BackendStyle = "rs"
	BackendStylePod         BackendStyle = "pod"
)

const (
	GroupTypeLocation   = "location"
	GroupTypeSecurity   = "Security"
	GroupTypeManagement = "Management"
	StatusProvisioned   = "Provisoned"
	StatusCompleted     = "Completed"
	StatusArchiving     = "Archiving"
	StatusRunning       = "Running"
	StatusProvisioning  = "Provisioning"
	StatusScheduled     = "Scheduled"
	StatusPending       = "Pending"
	StatusRetrying      = "Retrying"
	StatusFailed        = "Failed"
)

type ResourceAction string

// Define constants for supported resource types.
const (
	ResourceLockOn                ResourceAction = "AddOwnerReference"
	ResourceLabelAdd              ResourceAction = "LabelAdd"
	ResourceLabelDelete           ResourceAction = "LabelDelete"
	ResourceAnnotationInsert      ResourceAction = "Insert"
	ResourceAnnotationRemove      ResourceAction = "Remove"
	ResourceAnnotationInsertArray ResourceAction = "InsertArray"
	ResourceAnnotationRemoveArray ResourceAction = "RemoveArray"
	ResourceDelete                ResourceAction = "Delete"
)

const (
	LockOnKey         = "robokube.io/lockon"
	RoboKubeGroupKey  = "group.robokube.io/"
	RoboKubeModelKey  = "model.robokube.io"
	RoboKubeIndexKey  = "index.model.robokube.io/"
	RoboKubeEntityKey = "entity.robokube.io"
	RoboKubeTaskKey   = "task.robokube.io/"
	RoboKubeNsKey     = "robokube.io"
)

// Condition describes the state of a resource at a certain point.
type Condition struct {
	// Type of condition. such as Progressing/Ready/Reconciling
	Type string `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status string `json:"status"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// (Optional) Unique, this should be a short, machine understandable string that gives the reason
	// for the condition's last transition. For example, "ImageFound" could be used as the reason for
	// a resource's Status being "Running".
	Reason string `json:"reason,omitempty"`
	// (Optional) Human-readable message indicating details about last transition.
	Message string `json:"message,omitempty"`
}

// addToArrayUnique (modified to work with string arrays)
func AddToArrayUnique(array []string, valueToAdd string) []string {
	for _, existingValue := range array {
		if existingValue == valueToAdd {
			return array // Value already exists
		}
	}
	return append(array, valueToAdd)
}

// removeFromArray removes a specific value from a string array.
func RemoveFromArray(array []string, valueToRemove string) []string {
	newArray := []string{}
	for _, v := range array {
		if v != valueToRemove {
			newArray = append(newArray, v)
		}
	}
	return newArray
}

func BuildLabelObj(key string, val string, action ResourceAction) func(client.Object) {
	return func(targetPtr client.Object) {
		labels := targetPtr.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		if action == ResourceLabelDelete {
			delete(labels, key)
			targetPtr.SetLabels(labels)
			return
		}
		labels[key] = val
		targetPtr.SetLabels(labels)
	}
}

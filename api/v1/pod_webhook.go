package v1

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strconv"
	"strings"
	//"sigs.k8s.io/controller-runtime/pkg/webhook"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

/* todo:
1. mutate serviceAccount fro each pod
2. Check related entity and block if entity is marked delete
3. mutate pvc volume
*/

// log is for logging in this package.
var podlog = logf.Log.WithName("pod-resource")

// PodMutator mutates Pods
type podMutator struct {
	Client client.Client
}

func NewPodMutator(mgr manager.Manager) *podMutator {
	return &podMutator{
		Client: mgr.GetClient(),
	}
}

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create,versions=v1,name=mpod.kb.io,admissionReviewVersions=v1

func (m *podMutator) Handle(ctx context.Context, r admission.Request) admission.Response {
	podlog.Info("Handling pod mutation request", "namespace", r.Namespace, "name", r.Name)

	pod := &corev1.Pod{}
	if err := json.Unmarshal(r.Object.Raw, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode pod: %w", err))
	}

	// Get the StatefulSet that owns this Pod
	stsName, found := pod.GetOwnerReferences()[0].Name, len(pod.GetOwnerReferences()) > 0
	if !found {
		podlog.Info("Sts has no owner, skipping mutation", "namespace", r.Namespace, "name", r.Name)
		return admission.Allowed("Sts has no owner")
	}

	sts := &appsv1.StatefulSet{}
	if err := m.Client.Get(ctx, types.NamespacedName{Name: stsName, Namespace: r.Namespace}, sts); err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to get StatefulSet: %w", err))
	}

	// Get the Model associated with the StatefulSet
	modelName, found := sts.GetOwnerReferences()[0].Name, len(sts.GetOwnerReferences()) > 0
	if !found {
		podlog.Info("Sts has no owner, skipping mutation", "namespace", r.Namespace, "name", r.Name)
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("Sts has no owner"))
	}

	model := &Model{}
	if err := m.Client.Get(ctx, types.NamespacedName{Name: modelName, Namespace: r.Namespace}, model); err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to get Model: %w", err))
	}

	// Find the Entity associated with the Model and Pod name
	entity, found := m.findEntityForPod(ctx, model, pod.Name)
	if !found {
		podlog.Info("No Entity found for Pod, skipping mutation", "namespace", r.Namespace, "podName", pod.Name)
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("no Entity found for Pod"))
	}

	// Mutate the Pod annotations
	pod.Labels[RoboKubeIndexKey+model.Name] = fmt.Sprintf("%d", *entity.Spec.OrdinalIndex)
	pod.Labels[RoboKubeModelKey] = model.Name

	// Todo: apply the grouping label of the entity to the pod
	if err := m.syncGroupLabels(entity, pod); err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to sync group labels: %w", err))
	}

	// Mutate the Pod's service account
	// TODO: Implement your service account logic here
	// pod.Spec.ServiceAccountName = "your-service-account"

	pod.Spec.ServiceAccountName = entity.Name + "-sa"
	// Convert the modified Pod to JSON
	podJSON, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to marshal Pod to JSON: %w", err))
	}

	podlog.Info("Pod mutated successfully", "namespace", r.Namespace, "name", r.Name)
	return admission.PatchResponseFromRaw(r.Object.Raw, podJSON)
}

// syncGroupLabels syncs the group labels between the entity and the pod.
func (m *podMutator) syncGroupLabels(entity *Entity, pod *corev1.Pod) error {
	// Get the group labels from the entity
	groupLabels := make(map[string]string)
	for key, value := range entity.Labels {
		if strings.HasPrefix(key, RoboKubeGroupKey) {
			groupLabels[key] = value
		}
	}

	// Update the pod labels with the group labels
	for key, value := range groupLabels {
		pod.Labels[key] = value
	}

	// Remove group labels from the pod that are not present in the entity
	for key := range pod.Labels {
		if strings.HasPrefix(key, RoboKubeGroupKey) {
			if _, ok := groupLabels[key]; !ok {
				delete(pod.Labels, key)
			}
		}
	}

	return nil
}

// findEntityForPod finds the Entity associated with the given Model and Pod name.
func (m *podMutator) findEntityForPod(ctx context.Context, model *Model, podName string) (*Entity, bool) {
	entityList := &EntityList{}
	parts := strings.Split(podName, "-")
	sequenceNumber, _ := strconv.Atoi(parts[len(parts)-1])
	labelSelector := labels.Set{RoboKubeIndexKey + model.Name: fmt.Sprintf("%d", sequenceNumber)}.AsSelector()
	if err := m.Client.List(ctx, entityList, &client.ListOptions{
		Namespace:     model.Namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return nil, false
	}

	if len(entityList.Items) > 0 {
		return &entityList.Items[0], true
	}

	return nil, false
}

// PodValidator validates Pods
type PodValidator struct{}

func NewPodValidator(mgr manager.Manager) *PodValidator {
	// check if the Decommissioned and stop pod to be created

	return &PodValidator{}
}

// +kubebuilder:webhook:path=/validate-v1-pod,mutating=false,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=vpod.kb.io,admissionReviewVersions=v1

func (m *PodValidator) Handle(ctx context.Context, r admission.Request) admission.Response {
	return admission.Allowed("")
}

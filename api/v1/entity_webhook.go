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
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sort"
)

// log is for logging in this package.
var entitylog = logf.Log.WithName("entity-resource")

type entityValidator struct {
	Client client.Client
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *entityValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	entity, ok := obj.(*Entity)
	if !ok {
		return nil, fmt.Errorf("expected an Entity object, got %T", obj)
	}

	entitylog.Info("validate create", "name", entity.Name)

	// bypassed below logic if the entity has owner reference, meaning created by Model controller
	if len(entity.GetOwnerReferences()) > 0 {
		return nil, nil
	}

	// check if the ordinalIndex lables exits on other entity object
	labelSelector := labels.Set{RoboKubeIndexKey + entity.Spec.Model: fmt.Sprintf("%d", *entity.Spec.OrdinalIndex)}.AsSelector()
	entityList := &EntityList{}
	if err := v.Client.List(ctx, entityList, &client.ListOptions{
		Namespace:     entity.Namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return nil, fmt.Errorf("Entity creation rejected: cannot list another entities: %w", err)

	}

	if len(entityList.Items) > 0 {
		return nil, fmt.Errorf("Entity creation rejected: the OrdinalIndex already taken by another entity: %d", entity.Spec.OrdinalIndex)
	}

	model := &Model{}
	modelNamespacedName := types.NamespacedName{
		Namespace: entity.Namespace,
		Name:      entity.Spec.Model,
	}
	if err := v.Client.Get(ctx, modelNamespacedName, model); err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	// get sts based on Model's name and then get it's replicas number
	sts := &appsv1.StatefulSet{}
	stsName := entity.Spec.Model + "-sts"
	if model.Spec.StatefulSetName != "" {
		stsName = model.Spec.StatefulSetName
	}
	stsNamespacedName := types.NamespacedName{
		Namespace: entity.Namespace,
		Name:      stsName,
	}

	if err := v.Client.Get(ctx, stsNamespacedName, sts); err != nil {
		return nil, fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	// Get the replica count
	if *entity.Spec.OrdinalIndex > *sts.Spec.Replicas+int32(1) {
		return nil, fmt.Errorf("the index is out of range")
	}

	return nil, nil
}

func (v *entityValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	entity, ok := oldObj.(*Entity)
	if !ok {
		return nil, fmt.Errorf("expected an Entity object, got %T", newObj)
	}
	entitylog.Info("validate update", "name", entity.Name)

	return nil, nil

}

func (v *entityValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	entity, ok := obj.(*Entity)
	if !ok {
		return nil, fmt.Errorf("expected an Entity object, got %T", obj)
	}
	entitylog.Info("validate delete", "name", entity.Name)

	return nil, nil
}

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *Entity) SetupWebhookWithManager(mgr ctrl.Manager) error {
	v := &entityValidator{
		Client: mgr.GetClient(), // Get the client from the manager
	}
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithValidator(v).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// EntityMutator mutates Entities
type entityMutator struct {
	Client  client.Client
	decoder *admission.Decoder
}


func NewEntityMutator(mgr manager.Manager) *entityMutator {
	return &entityMutator{
		Client: mgr.GetClient(),
	}
}

// Handle implements admission.Handler
func (m *entityMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	entity := &Entity{}

	if err := json.Unmarshal(req.Object.Raw, entity); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode entity: %w", err))
	}

	// Set default SyncPeriod if not specified
	if entity.Spec.SyncPeriod <= 0 {
		entity.Spec.SyncPeriod = 60 // Default to 60 seconds
	}
	// set default identity with guid
	if entity.Spec.Identity == "" {
		entity.Spec.Identity = uuid.New().String()
	}

	if entity.Spec.OrdinalIndex == nil {
		// find the available ordinalIndex by checking the entityList
		entityList := &EntityList{}
		labelSelector, err := labels.Parse(RoboKubeIndexKey + entity.Spec.Model)
		if err != nil {
			admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to create label selector: %w", err))
		}
		if err := m.Client.List(ctx, entityList, &client.ListOptions{
			Namespace:     entity.Namespace,
			LabelSelector: labelSelector,
		}); err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to list: %w", err))
		}
		// find the available ordinalIndex, get the ordinalIndex from the entityList and sorted in ascending order
		// then find the first available ordinalIndex
		sort.SliceStable(entityList.Items, func(i, j int) bool {
			return *entityList.Items[i].Spec.OrdinalIndex < *entityList.Items[j].Spec.OrdinalIndex
		})
		ordinalIndex := int32(-1)
		maxOrdinalIndex := int32(0)
		// the first available ordinalIndex is the first not continuous ordinalIndex
		for _, obj := range entityList.Items {
			ordinalIndex++
			maxOrdinalIndex = *obj.Spec.OrdinalIndex
			if maxOrdinalIndex > ordinalIndex {
				break
			}
		}
		if maxOrdinalIndex == ordinalIndex || ordinalIndex == -1 {
			ordinalIndex++
		}
		entity.Spec.OrdinalIndex = &ordinalIndex
		entitylog.Info("found the available ordinalIndex", "name", entity.Name, "ordinalIndex", ordinalIndex)
	}
	// set label on the object with OrdinalIndex
	if entity.Labels == nil {
		entity.Labels = make(map[string]string)
	}
	entity.Labels[RoboKubeIndexKey+entity.Spec.Model] = fmt.Sprintf("%d", *entity.Spec.OrdinalIndex)

	marshaledEntity, err := json.Marshal(entity)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledEntity)
}



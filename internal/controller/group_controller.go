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
1. update entity label
2. create network policy based on security group
*/

package controller

import (
	"context"
	"fmt"
	"meliazeng/RoboKube/internal/controller/handler"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	//"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	robokubev1 "meliazeng/RoboKube/api/v1"
)

// GroupReconciler reconciles a Group object
type GroupReconciler struct {
	log logr.Logger
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=groups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=groups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=groups/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Group object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *GroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		res = robokubev1.Group{}
	)

	if err := r.Get(ctx, req.NamespacedName, &res); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("resource could not be found, %w", err))
	}

	if res.Status.ObservedGeneration == res.GetGeneration() && res.Status.Phase == robokubev1.StatusProvisioned {
		return reconcile.Result{}, nil
	}

	var (
		_, cancel = context.WithTimeout(ctx, time.Minute*1)
	)
	defer cancel()

	if result, err := handleFinalizer(ctx, r.Client, &res, finalizer); err != nil || result.Requeue {
		return result, err
	}

	if res.Status.Phase == "" {
		if err := UpdateStatus(ctx, r.Client, &res, updateStatusPhase(robokubev1.StatusProvisioning)); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set resource to progressing state, %w", err))
		}
	}

	if res.Status.Phase != robokubev1.StatusProvisioned {
		// Create NetworkPolicy
		netpolDeploymentClientProcessor := handler.NewClusterResourceHandler(&res, &handler.ClusterResourceParams{
			ResourceType: handler.ResourceTypeNetPol,
			TargetName:   res.Name + "-netpol",
			Namespace:    res.Namespace,
			NeedVerify:   false,
			Template:     "np-group.yml",
		}, r.Scheme)

		if _, err := Processor(ctx, r.Client, &res, nil, netpolDeploymentClientProcessor.Build, BuildNp(robokubev1.RoboKubeGroupKey+res.Name, "true"), netpolDeploymentClientProcessor.Handle, setCondition, r.log); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to create netpol, %w", err))
		}

		if err := UpdateStatus(ctx, r.Client, &res, updateStatusPhase(robokubev1.StatusProvisioned)); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set resource to progressing state, %w", err))
		}
	}

	// find the LastTarget and apply the label to the related entity
	if res.Spec.LastTargets != nil {
		// loop through LastTargets
		for _, lastTarget := range *res.Spec.LastTargets {
			entityList, err := GetEntityListForTarget(ctx, r.Client, res.Namespace, lastTarget)
			if err != nil {
				return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to get entity list: %w", err))
			}

			if err := r.updateLabelsForEntities(ctx, &res, entityList, lastTarget); err != nil {
				return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to update labels: %w", err))
			}
		}
	}

	// Search for entities with matching label
	labelSelector, err := labels.Parse(robokubev1.RoboKubeGroupKey + res.Name)
	if err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to create label selector"))
	}
	entityList := &robokubev1.EntityList{}
	if err := r.List(ctx, entityList, &client.ListOptions{
		Namespace:     res.Namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to list entities: %w", err))
	}

	// Update phase to Complete
	if err := UpdateStatus(ctx, r.Client, &res, func(obj client.Object) {
		if group, ok := obj.(*robokubev1.Group); ok {
			group.Status.TotalEntities = len(entityList.Items)
		}
	}, updateStatusPhase(robokubev1.StatusProvisioned)); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to update resource phase to Complete: %w", err))
	}

	r.log.Info("Successfully reconciled group %s", "name", res.Name)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.log = ctrl.Log.WithName("controllers").WithName("Group")
	return ctrl.NewControllerManagedBy(mgr).
		For(&robokubev1.Group{}).
		Complete(r)
}

func BuildNp(key string, val string) func(client.Object) {
	return func(obj client.Object) {
		target := obj.(*networkingv1.NetworkPolicy)
		//target.Spec.PodSelector.MatchLabels[key] = val
		for i := range target.Spec.Egress {
			// Assuming you want to modify the first NetworkPolicyPeer's PodSelector
			if len(target.Spec.Egress[i].To) > 0 {
				if target.Spec.Egress[i].To[0].PodSelector != nil {
					// Modify existing MatchLabels
					target.Spec.Egress[i].To[0].PodSelector.MatchExpressions[0].Key = key
				}
			}
		}
		for i := range target.Spec.Ingress {
			if len(target.Spec.Ingress[i].From) > 0 {
				if target.Spec.Ingress[i].From[0].PodSelector != nil {
					target.Spec.Ingress[i].From[0].PodSelector.MatchExpressions[0].Key = key
				}
			}
		}
	}
}

func GetEntityListForTarget(ctx context.Context, r client.Client, namespace string, lastTarget robokubev1.TargetRef) (*robokubev1.EntityList, error) {
	entityList := &robokubev1.EntityList{}
	switch lastTarget.Type {
	case robokubev1.TargetTypeEntity:
		entity := &robokubev1.Entity{}
		entityNamespacedName := types.NamespacedName{
			Namespace: namespace,
			Name:      lastTarget.Name,
		}

		if err := r.Get(ctx, entityNamespacedName, entity); err != nil {
			return nil, fmt.Errorf("failed to get entity: %w", err)
		}

		entityList.Items = append(entityList.Items, *entity)

	case robokubev1.TargetTypeGroup:
		labelSelector, err := labels.Parse(robokubev1.RoboKubeGroupKey + lastTarget.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to create label selector")
		}
		if err := r.List(ctx, entityList, &client.ListOptions{
			Namespace:     namespace,
			LabelSelector: labelSelector,
		}); err != nil {
			return nil, fmt.Errorf("failed to list entities: %w", err)
		}

	case robokubev1.TargetTypeModel:
		labelSelector, err := labels.Parse(robokubev1.RoboKubeIndexKey + lastTarget.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to create label selector")
		}
		if err := r.List(ctx, entityList, &client.ListOptions{
			Namespace:     namespace,
			LabelSelector: labelSelector,
		}); err != nil {
			return nil, fmt.Errorf("failed to list entities: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported target type: %s", lastTarget.Type)
	}
	return entityList, nil
}

func (r *GroupReconciler) updateLabelsForEntities(ctx context.Context, res *robokubev1.Group, entityList *robokubev1.EntityList, lastTarget robokubev1.TargetRef) error {
	labelClientProcessor := handler.NewObjectHandler(res, &handler.ObjectParams{
		ResourceType: handler.ResourceTypeEntity,
		Action:       "Update Label",
	})
	for i := range entityList.Items {
		// update label on entity
		if _, err := Processor(ctx, r.Client, res, &entityList.Items[i], nil, robokubev1.BuildLabelObj(robokubev1.RoboKubeGroupKey+res.Name, entityList.Items[i].Spec.Identity, lastTarget.Action), labelClientProcessor.Handle, setCondition, r.log); err != nil {
			return fmt.Errorf("failed to update label on entity: %w", err)
		}

		if entityList.Items[i].Status.Phase != robokubev1.StatusProvisioned {
			continue
		}

		// update label on pod
		pod := &corev1.Pod{}
		podNamespacedName := types.NamespacedName{
			Namespace: entityList.Items[i].Namespace,
			Name:      entityList.Items[i].Status.PodName,
		}

		if err := r.Get(ctx, podNamespacedName, pod); err != nil {
			return fmt.Errorf("failed to get pod: %w", err)
		}

		if _, err := Processor(ctx, r.Client, res, pod, nil, robokubev1.BuildLabelObj(robokubev1.RoboKubeGroupKey+res.Name, entityList.Items[i].Spec.Identity, lastTarget.Action), labelClientProcessor.Handle, setCondition, r.log); err != nil {
			return fmt.Errorf("failed to update label on pod: %w", err)
		}
	}
	return nil
}

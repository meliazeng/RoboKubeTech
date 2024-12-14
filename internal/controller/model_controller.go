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

package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	k8sErr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	//"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	//"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"github.com/go-logr/logr"
	//networkingv1 "k8s.io/api/networking/v1"
	robokubev1 "meliazeng/RoboKube/api/v1"
	"meliazeng/RoboKube/internal/controller/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

// ModelReconciler reconciles a Model object
type ModelReconciler struct {
	log logr.Logger
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=models,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=models/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=models/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=entities,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;update;patch;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Model object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *ModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		res = robokubev1.Model{}
		//log = log.FromContext(ctx).WithValues("modelController", "")
	)
	var (
		_, cancel = context.WithTimeout(ctx, time.Minute*1)
	)
	defer cancel()

	if err := r.Get(ctx, req.NamespacedName, &res); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("resource could not be found, %w", err))
	}

	var (
		modelLabel = map[string]string{
			robokubev1.RoboKubeModelKey: res.Name,
		}
	)

	if result, err := handleFinalizer(ctx, r.Client, &res, finalizer); err != nil || result.Requeue {
		return result, err
	}

	if res.Status.Phase == "" {
		if err := UpdateStatus(ctx, r.Client, &res, updateStatusPhase(robokubev1.StatusProvisioning)); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set resource to progressing state, %w", err))
		}
	}

	var currentReplicas int32 = 0
	sts := &appsv1.StatefulSet{}
	stsName := res.Name + "-sts"
	if res.Spec.StatefulSetName != "" {
		stsName = res.Spec.StatefulSetName
	}
	if err := r.Get(ctx, types.NamespacedName{Namespace: res.Namespace, Name: stsName}, sts); err != nil {
		if !k8sErr.IsNotFound(err) {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to get sts: %w", err))
		}
	} else {
		currentReplicas = *sts.Spec.Replicas
	}

	if res.Status.ObservedGeneration == res.GetGeneration() && res.Status.Phase == robokubev1.StatusProvisioned {
		if err := UpdateStatus(ctx, r.Client, &res, func(obj client.Object) {
			res.Status.CurrentReplicas = fmt.Sprintf("%d/%d", currentReplicas, *res.Spec.TargetReplicas)
		}); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set resource to progressing state, %w", err))
		}
		return reconcile.Result{}, nil
	} else if res.Status.Phase == robokubev1.StatusProvisioned {
		if err := UpdateStatus(ctx, r.Client, &res, updateStatusPhase(robokubev1.StatusProvisioning)); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set resource to progressing state, %w", err))
		}
	}

	if res.Status.Phase != robokubev1.StatusProvisioned {

		netpolDeploymentClientProcessor := handler.NewClusterResourceHandler(&res, &handler.ClusterResourceParams{
			ResourceType: handler.ResourceTypeNetPol,
			TargetName:   "allow-operator-ingress",
			Namespace:    res.Namespace,
			NeedVerify:   false,
			Template:     "np-operator.yml",
		}, r.Scheme)

		if _, err := Processor(ctx, r.Client, &res, nil, netpolDeploymentClientProcessor.Build, func(client.Object) {}, netpolDeploymentClientProcessor.Handle, setCondition, r.log); err != nil {
			return ctrl.Result{}, err
		}

		// check if ns label exists
		ns := &corev1.Namespace{}
		if err := r.Get(ctx, types.NamespacedName{Name: res.Namespace}, ns); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to get namespace: %w", err))
		}

		if _, ok := ns.Labels[robokubev1.RoboKubeNsKey]; !ok {
			LabelClientProcessor := handler.NewObjectHandler(&res, &handler.ObjectParams{
				ResourceType: handler.ResourceTypeEntity,
				Action:       "Update Label",
			})
			if _, err := Processor(ctx, r.Client, &res, ns, nil, robokubev1.BuildLabelObj(robokubev1.RoboKubeNsKey, "true", robokubev1.ResourceLabelAdd), LabelClientProcessor.Handle, setCondition, r.log); err != nil {
				return ctrl.Result{}, err
			}
		}

		serviceName := res.Name + "-srv"
		if res.Spec.StatefulSetName != "" {
			serviceName = res.Spec.StatefulSetName
		}
		// create service
		SrvDeploymentClientProcessor := handler.NewClusterResourceHandler(&res, &handler.ClusterResourceParams{
			ResourceType:       handler.ResourceTypeService,
			TargetName:         serviceName,
			Namespace:          res.Namespace,
			InjectLabels:       modelLabel,
			SetOwnerReferences: true,
			Template:           "srv.yml",
		}, r.Scheme)

		if _, err := Processor(ctx, r.Client, &res, nil, SrvDeploymentClientProcessor.Build, func(obj client.Object) {
			target := obj.(*corev1.Service)
			target.Name = serviceName
			if target.Spec.Selector == nil {
				target.Spec.Selector = make(map[string]string)
			}
			target.Spec.Selector[robokubev1.RoboKubeModelKey] = res.Name
		}, SrvDeploymentClientProcessor.Handle, setCondition, r.log); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.manageEntities(ctx, &res, currentReplicas, *res.Spec.TargetReplicas); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to manage entities: %w", err))
	}

	// Create sts - will be call again when scale up.
	if res.Spec.StatefulSetName != "" {
		sts := &appsv1.StatefulSet{}
		if err := r.Get(ctx, types.NamespacedName{Name: res.Spec.StatefulSetName, Namespace: res.Namespace}, sts); err != nil {
			// No suitable sts found, requeue
			r.log.Info("No suitable Sts found, requeuing", "Statefulset", sts.Name)
			return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
		}

		// set ownership
		EntityClientProcessor := handler.NewObjectHandler(&res, &handler.ObjectParams{
			ResourceType: handler.ResourceTypeStatefulSet,
			Action:       "Set ownership",
		})

		if _, err := Processor(ctx, r.Client, sts, nil, nil, func(o client.Object) {
			ctrl.SetControllerReference(&res, o, r.Client.Scheme())
			if sts, ok := o.(*appsv1.StatefulSet); ok {
				if sts.Labels == nil {
					sts.Labels = make(map[string]string)
				}
				sts.Labels[robokubev1.RoboKubeModelKey] = res.Name
				*sts.Spec.Replicas = *res.Spec.TargetReplicas
			}
		}, EntityClientProcessor.Handle, setCondition, r.log); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set ownership for entities: %w", err))
		}
	} else {
		StsDeploymentClientProcessor := handler.NewClusterResourceHandler(&res, &handler.ClusterResourceParams{
			ResourceType:       handler.ResourceTypeStatefulSet,
			TargetName:         res.Name + "-sts",
			SetOwnerReferences: true,
			Namespace:          res.Namespace,
			InjectLabels:       modelLabel,
			Template:           "sts.yml",
		}, r.Scheme)

		if _, err := Processor(ctx, r.Client, &res, nil, StsDeploymentClientProcessor.Build, func(obj client.Object) {
			target := obj.(*appsv1.StatefulSet)
			target.Spec.Replicas = res.Spec.TargetReplicas
			target.Spec.Template.Spec.Containers[0].Image = res.Spec.DockerImage
			target.Spec.Selector.MatchLabels[robokubev1.RoboKubeModelKey] = res.Name
			target.Spec.Template.ObjectMeta.Labels[robokubev1.RoboKubeModelKey] = res.Name
		}, StsDeploymentClientProcessor.Handle, setCondition, r.log); err != nil {
			return ctrl.Result{}, err
		}
	}

	r.log.Info("Successfully reconciled Model", "modelName", res.Name)

	if err := UpdateStatus(ctx, r.Client, &res, func(obj client.Object) {
		res.Status.CurrentReplicas = fmt.Sprintf("%d/%d", *res.Spec.TargetReplicas, *res.Spec.TargetReplicas)
	}, updateStatusPhase(robokubev1.StatusProvisioned)); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set resource to progressing state, %w", err))
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.log = ctrl.Log.WithName("controllers").WithName("Model")
	return ctrl.NewControllerManagedBy(mgr).
		For(&robokubev1.Model{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}

func (r *ModelReconciler) manageEntities(ctx context.Context, res *robokubev1.Model, currentReplicas, targetReplicas int32) error {
	entityList := &robokubev1.EntityList{}
	labelSelector, err := labels.Parse(robokubev1.RoboKubeIndexKey + res.Name)
	if err != nil {
		return fmt.Errorf("failed to create label selector: %w", err)
	}
	if err := r.List(ctx, entityList, &client.ListOptions{
		Namespace:     res.Namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return fmt.Errorf("failed to list entities: %w", err)
	}

	entityMap := make(map[int32]*robokubev1.Entity)
	for i := range entityList.Items {
		entity := &entityList.Items[i]
		if entity.Spec.OrdinalIndex != nil {
			entityMap[*entity.Spec.OrdinalIndex] = entity
		}
	}

	if currentReplicas < targetReplicas {
		for i := currentReplicas; i < targetReplicas; i++ {
			if _, exists := entityMap[i]; exists {
				continue
			}

			EntityDeploymentClientProcessor := handler.NewClusterResourceHandler(res, &handler.ClusterResourceParams{
				ResourceType:       handler.ResourceTypeEntity,
				SetOwnerReferences: true,
				TargetName:         fmt.Sprintf("%s-%d", res.Name, i),
				Namespace:          res.Namespace,
				InjectLabels: map[string]string{
					robokubev1.RoboKubeIndexKey + res.Name: fmt.Sprintf("%d", i),
				},
				Template: "entity.yml",
			}, r.Scheme)

			if _, err := Processor(ctx, r.Client, res, nil, EntityDeploymentClientProcessor.Build, func(obj client.Object) {
				target := obj.(*robokubev1.Entity)
				target.Spec.Model = res.GetName()
				target.Spec.OrdinalIndex = &i
				target.Spec.Usage = "created during model Initialization"
			}, EntityDeploymentClientProcessor.Handle, setCondition, r.log); err != nil {
				return err
			}
		}
	} else if currentReplicas > targetReplicas {
		EntityClientProcessor := handler.NewObjectHandler(res, &handler.ObjectParams{
			ResourceType: handler.ResourceTypeEntity,
			Action:       "Delete Entity",
			IsDelete:     true,
		})
		for i := targetReplicas; i < currentReplicas; i++ {
			if entity, exists := entityMap[i]; exists {
				r.Recorder.Event(res, "Normal", "Delete Entity",
					fmt.Sprintf("Entity %s is being deleted due to scale down", entity.Name))

				if _, err := Processor(ctx, r.Client, res, entity, nil, nil, EntityClientProcessor.Handle, setCondition, r.log); err != nil {
					return fmt.Errorf("entity deletion failed: %w", err)
				}
			}
		}
	}

	return nil
}

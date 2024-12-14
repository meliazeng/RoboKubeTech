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
1. locate the new pod and add label to the pod.
2. Create cert from cert manager
3. Sync pod status.
4. Schedule for the next sync
5. offboard process
*/

package controller

import (
	"context"
	"fmt"

	//"strings"
	"time"

	robokubev1 "meliazeng/RoboKube/api/v1"
	"meliazeng/RoboKube/internal/controller/handler"

	certmanager "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	//v1 "k8s.io/client-go/applyconfigurations/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// EntityReconciler reconciles a Entity object
type EntityReconciler struct {
	log logr.Logger
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

const (
	finalizer = "robokube.meliazeng.github.io/finalizer"
)

// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=entities,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=entities/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=robokube.meliazeng.github.io,resources=entities/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;create;update;watch;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Entity object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *EntityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		res = robokubev1.Entity{}
		//log = log.FromContext(ctx).WithValues("entityController", "") //.WithCallDepth(1)
	)

	if err := r.Get(ctx, req.NamespacedName, &res); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("resource could not be found, %w", err))
	}
	var (
		entityLabel = map[string]string{
			robokubev1.RoboKubeEntityKey: res.Name,
		}
	)

	var (
		_, cancel = context.WithTimeout(ctx, time.Minute*1)
	)
	defer cancel()

	if result, err := handleFinalizer(ctx, r.Client, &res, finalizer); err != nil || result.Requeue {
		return result, err
	}

	if res.Status.ObservedGeneration == res.GetGeneration() && res.Status.Phase == robokubev1.StatusProvisioned {
		// process to call pod status and update the condition
		EntityStatusClientProcessor := handler.NewRequestHandler(nil, res.Status.Urls["status"], "get", handler.RequestParams{
			ResourceType:  handler.ResourceTypePod,
			TargetName:    res.Status.PodName,
			ErrorCondtion: false,
		})

		// call status endpoint
		if _, err := Processor(ctx, r.Client, &res, nil, nil, nil, EntityStatusClientProcessor.Handle, SetStatusData, r.log); err != nil {
			return ctrl.Result{}, err
		}

		return reconcile.Result{RequeueAfter: time.Duration(res.Spec.SyncPeriod) * time.Second}, nil
	}

	if res.Status.Phase == "" || res.Status.ObservedGeneration != res.GetGeneration() {
		if err := UpdateStatus(ctx, r.Client, &res, updateStatusPhase(robokubev1.StatusProvisioning)); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to set resource to progressing state, %w", err))
		}
	}

	// Lock on process
	// 3. check the replicas of the sts, increase its replicas to match ordinalIndex
	// 4. in entity controller to set ownership of Model
	// 5. raise events to Model to trigger reconile to update the status field for total number of entity.
	// 1. Get the associated Model
	model := &robokubev1.Model{}
	if err := r.Get(ctx, types.NamespacedName{Name: res.Spec.Model, Namespace: res.Namespace}, model); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to get Model: %w", err))
	}

	// create a  service account
	SADeploymentClientProcessor := handler.NewClusterResourceHandler(&res, &handler.ClusterResourceParams{
		ResourceType:       handler.ResourceTypeServiceAccount,
		TargetName:         res.Name + "-sa",
		Namespace:          res.Namespace,
		SetOwnerReferences: true,
		InjectLabels:       entityLabel,
		Template:           "sa.yml",
	}, r.Scheme)

	if _, err := Processor(ctx, r.Client, &res, nil, SADeploymentClientProcessor.Build, func(client.Object) {}, SADeploymentClientProcessor.Handle, setCondition, r.log); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to create service account: %w", err))
	}

	// add service Account to rolebinding if it is priviliaged
	if res.Spec.Privileged {
		// Create/update rolebinding for privileged entities
		RoleBindingProcessor := handler.NewClusterResourceHandler(&res, &handler.ClusterResourceParams{
			ResourceType:       handler.ResourceTypeRoleBinding,
			TargetName:         res.Name + "-rb",
			Namespace:          res.Namespace,
			SetOwnerReferences: true,
			InjectLabels:       entityLabel,
			Template:           "rb.yml",
		}, r.Scheme)

		if _, err := Processor(ctx, r.Client, &res, nil, RoleBindingProcessor.Build, func(obj client.Object) {
			target := obj.(*rbacv1.RoleBinding)
			target.Subjects = []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      res.Name + "-sa",
					Namespace: res.Namespace,
				},
			}
		}, RoleBindingProcessor.Handle, setCondition, r.log); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to create/update rolebinding: %w", err))
		}
	}

	secretName := res.Spec.Secret
	if res.Spec.Secret == "" {
		// creat a certificate for the entity
		CrtDeploymentClientProcessor := handler.NewClusterResourceHandler(&res, &handler.ClusterResourceParams{
			ResourceType:       handler.ResourceTypeCert,
			TargetName:         res.Name + "-crt",
			Namespace:          res.Namespace,
			SetOwnerReferences: true,
			InjectLabels:       entityLabel,
			Template:           "crt.yml",
		}, r.Scheme)

		if _, err := Processor(ctx, r.Client, &res, nil, CrtDeploymentClientProcessor.Build, func(obj client.Object) {
			target := obj.(*certmanager.Certificate)
			target.Spec.SecretName = res.Name + "-crt"
			target.Spec.CommonName = res.Name + "." + model.Spec.IngressUrl
		}, CrtDeploymentClientProcessor.Handle, setCondition, r.log); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to create certificate: %w", err))
		}
		secretName = res.Name + "-crt"
	}

	// create pod service
	SrvDeploymentClientProcessor := handler.NewClusterResourceHandler(&res, &handler.ClusterResourceParams{
		ResourceType:       handler.ResourceTypeService,
		TargetName:         res.Name + "-srv",
		Namespace:          res.Namespace,
		SetOwnerReferences: true,
		InjectLabels:       entityLabel,
		Template:           "srv-pod.yml",
	}, r.Scheme)

	if _, err := Processor(ctx, r.Client, &res, nil, SrvDeploymentClientProcessor.Build, func(obj client.Object) {
		target := obj.(*corev1.Service)
		if target.Spec.Selector == nil {
			target.Spec.Selector = make(map[string]string)
		}
		target.Spec.Selector[robokubev1.RoboKubeIndexKey+model.Name] = fmt.Sprintf("%d", *res.Spec.OrdinalIndex)
	}, SrvDeploymentClientProcessor.Handle, setCondition, r.log); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to create service: %w", err))
	}

	// create pod ingress
	IngDeploymentClientProcessor := handler.NewClusterResourceHandler(&res, &handler.ClusterResourceParams{
		ResourceType:       handler.ResourceTypeIngress,
		TargetName:         res.Name + "-ing",
		Namespace:          res.Namespace,
		SetOwnerReferences: true,
		InjectLabels:       entityLabel,
		Template:           "ing-pod.yml",
	}, r.Scheme)

	if _, err := Processor(ctx, r.Client, &res, nil, IngDeploymentClientProcessor.Build, func(obj client.Object) {
		target := obj.(*networkingv1.Ingress)
		target.Spec.TLS[0].SecretName = secretName
		target.Spec.TLS[0].Hosts[0] = res.Name + "." + model.Spec.IngressUrl
		target.Spec.Rules[0].Host = res.Name + "." + model.Spec.IngressUrl
		target.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name = res.Name + "-srv"
	}, IngDeploymentClientProcessor.Handle, setCondition, r.log); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to create ingress: %w", err))
	}

	// check if ownerreference, if it is not, raise an event to Model controller
	if len(res.GetOwnerReferences()) == 0 {
		if err := r.handleOwnershipAndReplicas(ctx, &res, model); err != nil {
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to handle ownership and replicas: %w", err))
		}
	}

	if res.Status.PodName == "" {
		if err := r.lockOnPod(ctx, &res, model); err != nil {
			if err.Error() == fmt.Sprintf("no suitable Pod found for Entity %s", res.Name) {
				// No suitable Pod found, requeue
				r.log.Info("No suitable Pod found, requeuing", "Entity", res.Name)
				return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
			}
			return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to lock on pod: %w", err))
		}
	}

	if err := UpdateStatus(ctx, r.Client, &res, updateStatusPhase(robokubev1.StatusProvisioned)); err != nil {
		return ErrorDelegateHandler(ctx, r.Client, &res, fmt.Errorf("failed to update status for entities: %w", err))
	}

	return ctrl.Result{RequeueAfter: time.Duration(res.Spec.SyncPeriod) * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EntityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.log = ctrl.Log.WithName("controllers").WithName("Entity")
	return ctrl.NewControllerManagedBy(mgr).
		For(&robokubev1.Entity{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 10,
		}).
		WithEventFilter(predicate.Or(
			predicate.GenerationChangedPredicate{},
		)).
		Complete(r)
}

func (r *EntityReconciler) lockOnPod(ctx context.Context, res *robokubev1.Entity, model *robokubev1.Model) error {
	log := log.FromContext(ctx)

	// Search for pods with matching label
	labelSelector := labels.Set{robokubev1.RoboKubeIndexKey + model.Name: fmt.Sprintf("%d", *res.Spec.OrdinalIndex)}.AsSelector()

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, &client.ListOptions{
		Namespace:     res.Namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return fmt.Errorf("failed to list pod: %w", err)
	}

	// find the first one of the list
	var targetPod *corev1.Pod
	if len(podList.Items) > 0 {
		// find any pod has the same name as the entity
		for i := range podList.Items {
			if podList.Items[i].Name == res.Name {
				targetPod = &podList.Items[i]
				break
			}
		}
		if targetPod == nil {
			targetPod = &podList.Items[0]
		}
	} else {
		// No suitable Pod found, return error to requeue
		return fmt.Errorf("no suitable Pod found for Entity %s", res.Name)
	}

	r.Recorder.Event(res, "Normal", "LockedOn",
		fmt.Sprintf("Entity %s is successfully locked on to pod %s", res.Name, targetPod.Name))
	log.Info("pod locked on successfully for entity", "name", res.Name, "pod", targetPod.Name)
	serviceName := res.Spec.Model + "-srv"
	if model.Spec.StatefulSetName != "" {
		serviceName = model.Spec.StatefulSetName
	}
	if err := UpdateStatus(ctx, r.Client, res, func(obj client.Object) {
		target := obj.(*robokubev1.Entity)
		target.Status.Urls = map[string]string{
			"status":  fmt.Sprintf("http://%s.%s.%s.svc.cluster.local/status", targetPod.Name, serviceName, res.Namespace),
			"task":    fmt.Sprintf("http://%s.%s.%s.svc.cluster.local/task", targetPod.Name, serviceName, res.Namespace),
			"control": fmt.Sprintf("http://%s.%s.%s.svc.cluster.local:8080/", targetPod.Name, serviceName, res.Namespace),
		}
		target.Status.PodName = targetPod.Name
	}); err != nil {
		return fmt.Errorf("failed to update Entity status to Provisioned: %w", err)
	}

	return nil
}

func (r *EntityReconciler) handleOwnershipAndReplicas(ctx context.Context, res *robokubev1.Entity, model *robokubev1.Model) error {
	//update sts and increase its replicas
	sts := &appsv1.StatefulSet{}
	stsName := res.Spec.Model + "-sts"
	if model.Spec.StatefulSetName != "" {
		stsName = model.Spec.StatefulSetName
	}
	stsNamespacedName := types.NamespacedName{
		Namespace: res.Namespace,
		Name:      stsName,
	}

	if err := r.Get(ctx, stsNamespacedName, sts); err != nil {
		return fmt.Errorf("failed to get sts: %w", err)
	}

	if *sts.Spec.Replicas < *res.Spec.OrdinalIndex {
		return fmt.Errorf("entity's index is not the next available replicas")
	}

	if *sts.Spec.Replicas == *res.Spec.OrdinalIndex {
		// update the sts replicas
		StsClientProcessor := handler.NewObjectHandler(res, &handler.ObjectParams{
			ResourceType: handler.ResourceTypeStatefulSet,
			Action:       "Increase replicas",
		})
		r.Recorder.Event(res, "Normal", "LockedOn",
			fmt.Sprintf("Entity %s request statefulset %s to increase replicas", res.Name, stsName))

		if _, err := Processor(ctx, r.Client, res, sts, nil, func(obj client.Object) {
			*(obj.(*appsv1.StatefulSet).Spec.Replicas) = *(obj.(*appsv1.StatefulSet).Spec.Replicas) + int32(1)
		}, StsClientProcessor.Handle, setCondition, r.log); err != nil {
			return fmt.Errorf("statefulset upgrade failed: %w", err)
		}
	}

	// set ownership
	EntityClientProcessor := handler.NewObjectHandler(res, &handler.ObjectParams{
		ResourceType: handler.ResourceTypeEntity,
		Action:       "Set ownership",
	})

	if _, err := Processor(ctx, r.Client, res, nil, nil, func(o client.Object) {
		ctrl.SetControllerReference(model, o, r.Client.Scheme())
	}, EntityClientProcessor.Handle, setCondition, r.log); err != nil {
		return fmt.Errorf("failed to set ownership for entities: %w", err)
	}

	return nil
}

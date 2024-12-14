package handler

import (
	"context"
	"fmt"
	certmanager "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	robokubev1 "meliazeng/RoboKube/api/v1"
	"os"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceType represents the type of Kubernetes resource.
type ResourceType string

// Define constants for supported resource types.
const (
	ResourceTypeEntity         ResourceType = "Entity"
	ResourceTypeStatefulSet    ResourceType = "StatefulSet"
	ResourceTypeDeployment     ResourceType = "Deployment"
	ResourceTypeReplicaSet     ResourceType = "ReplicaSet"
	ResourceTypePod            ResourceType = "Pod"
	ResourceTypeNamespace      ResourceType = "Namespace"
	ResourceTypeIngress        ResourceType = "Ingress"
	ResourceTypeNetPol         ResourceType = "NetworkPolicy"
	ResourceTypeCert           ResourceType = "Certificate"
	ResourceTypeCronJob        ResourceType = "CronJob"
	ResourceTypeNS             ResourceType = "Namespace"
	ResourceTypeService        ResourceType = "Service"
	ResourceTypeServiceAccount ResourceType = "ServiceAccount"
	ResourceTypeRoleBinding    ResourceType = "RoleBinding"
	TemplatePath               string       = "../internal/controller/template"
)

// Result represents the result of a resource operation.
type Result struct {
	ResourceName string       `json:"resourceName"`
	ResourceType ResourceType `json:"resourceType"`
	Reason       string       `json:"reason"`
	Message      string       `json:"message"`
	Status       string       `json:"status"`
}

type ClusterResourceParams struct {
	ResourceType       ResourceType
	TargetName         string
	Namespace          string
	InjectLabels       map[string]string
	NeedVerify         bool
	SetOwnerReferences bool
	Template           string
}

// ClusterResourceHandler handles operations for a generic Kubernetes resource.
type ClusterResourceHandler struct {
	ResourceType       ResourceType
	TargetName         string
	Namespace          string
	InjectLabels       map[string]string
	ParentObject       client.Object
	InitObject         client.Object
	NeedVerify         bool
	TemplateName       string
	SetOwnerReferences bool
	scheme             *runtime.Scheme
}

func NewClusterResourceHandler(parent client.Object, paras *ClusterResourceParams, scheme *runtime.Scheme) *ClusterResourceHandler {
	var empty client.Object
	switch paras.ResourceType {
	case ResourceTypeEntity:
		empty = &robokubev1.Entity{}
	case ResourceTypeStatefulSet:
		empty = &appsv1.StatefulSet{}
	case ResourceTypeDeployment:
		empty = &appsv1.Deployment{}
	case ResourceTypeReplicaSet:
		empty = &appsv1.ReplicaSet{}
	case ResourceTypePod:
		empty = &corev1.Pod{}
	case ResourceTypeNamespace:
		empty = &corev1.Namespace{}
	case ResourceTypeIngress:
		empty = &networkingv1.Ingress{}
	case ResourceTypeNetPol:
		empty = &networkingv1.NetworkPolicy{}
	case ResourceTypeCert:
		empty = &certmanager.Certificate{}
	case ResourceTypeCronJob:
		empty = &batchv1.CronJob{}
	case ResourceTypeService:
		empty = &corev1.Service{}
	case ResourceTypeServiceAccount:
		empty = &corev1.ServiceAccount{}
	case ResourceTypeRoleBinding:
		empty = &rbacv1.RoleBinding{}
	default:
		panic(fmt.Sprintf("unsupported resource type: %s", paras.ResourceType)) // Or handle the error gracefully
	}
	return &ClusterResourceHandler{
		ResourceType:       paras.ResourceType,
		TargetName:         paras.TargetName,
		Namespace:          paras.Namespace,
		InjectLabels:       paras.InjectLabels,
		NeedVerify:         paras.NeedVerify,
		ParentObject:       parent,
		InitObject:         empty,
		TemplateName:       paras.Template,
		SetOwnerReferences: paras.SetOwnerReferences,
		scheme:             scheme,
	}
}

func (h *ClusterResourceHandler) Build(obj *client.Object, f func(client.Object)) (client.Object, error) {
	if h.TemplateName == "" {
		return nil, fmt.Errorf("template name is empty")
	}

	codec := serializer.NewCodecFactory(h.scheme)
	decoder := codec.UniversalDeserializer()
	templateBytes, err := os.ReadFile(fmt.Sprintf("%s/%s", TemplatePath, h.TemplateName))
	if err != nil {
		return nil, err
	}
	object, _, err := decoder.Decode(templateBytes, nil, nil)
	if err != nil {
		return nil, err
	}

	// Type Assertion and Switch
	switch typedObj := object.(type) {
	case *robokubev1.Entity:
		f(typedObj)
		return typedObj, nil
	case *appsv1.StatefulSet:
		f(typedObj)
		return typedObj, nil
	case *appsv1.Deployment:
		f(typedObj)
		return typedObj, nil
	case *appsv1.ReplicaSet:
		f(typedObj)
		return typedObj, nil
	case *corev1.Pod:
		f(typedObj)
		return typedObj, nil
	case *corev1.Namespace:
		f(typedObj)
		return typedObj, nil
	case *networkingv1.Ingress:
		f(typedObj)
		return typedObj, nil
	case *networkingv1.NetworkPolicy:
		f(typedObj)
		return typedObj, nil
	case *certmanager.Certificate:
		f(typedObj)
		return typedObj, nil
	case *batchv1.CronJob:
		f(typedObj)
		return typedObj, nil
	case *corev1.Service:
		f(typedObj)
		return typedObj, nil
	case *corev1.ServiceAccount:
		f(typedObj)
		return typedObj, nil
	case *rbacv1.RoleBinding:
		f(typedObj)
		return typedObj, nil
	default:
		return nil, fmt.Errorf("unsupported resource type: %T", object)
	}
}

// Handle processes the operation for the specified resource type.
func (h *ClusterResourceHandler) Handle(ctx context.Context, client client.Client, obj client.Object, f func(client.Object)) (*Result, error) {
	// ... (Logic to create/update the Kubernetes resource based on h.ResourceType and h.TargetName)
	// You'll likely use client.Client to interact with the Kubernetes API here.

	/* todo:
	1. update annotation/labels
	2. Set ownership for the object
	3. Create or update object
	4. check if varify to wait
	*/
	if h.InjectLabels != nil {
		// 1. Add sample labels and annotations

		// Get existing labels and annotations
		objLabels := obj.GetLabels()
		if objLabels == nil {
			objLabels = make(map[string]string)
		}

		// Merge with new labels
		for k, v := range h.InjectLabels {
			objLabels[k] = v
		}

		obj.SetLabels(objLabels)
	}

	obj.SetName(h.TargetName)
	obj.SetNamespace(h.Namespace)
	// 2. Add reference ownership to the source
	// Set owner reference to the Parent
	if h.SetOwnerReferences {
		if err := ctrl.SetControllerReference(h.ParentObject, obj, client.Scheme()); err != nil {
			return &Result{
				ResourceName: h.TargetName,
				Message:      fmt.Sprintf("Faile to set owner reference to %s.", h.TargetName),
				Status:       "Failed",
				Reason:       "Owner Reference failed",
				ResourceType: h.ResourceType,
			}, err
		}
	}

	if err := client.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: h.TargetName}, h.InitObject); err != nil {
		// the object will be new
		if err := client.Create(ctx, obj); err != nil {
			return &Result{
				ResourceName: h.TargetName,
				Message:      fmt.Sprintf("Failed to create new Object %s.", h.TargetName),
				Status:       "Failed",
				Reason:       "Create Object failed",
				ResourceType: h.ResourceType,
			}, err
		}
	} else {
		// the object exists
		h.InitObject.SetLabels(obj.GetLabels())
		h.InitObject.SetAnnotations(obj.GetAnnotations())
		h.InitObject.SetOwnerReferences(obj.GetOwnerReferences())
		// 1. Type assert and update Spec based on ResourceType
		var err error
		switch h.ResourceType {
		case ResourceTypeEntity:
			if entity, ok := h.InitObject.(*robokubev1.Entity); ok {
				if reflect.DeepEqual(entity.Spec, obj.(*robokubev1.Entity).Spec) {
					return nil, nil
				}
				entity.Spec = *obj.(*robokubev1.Entity).Spec.DeepCopy()
			} else {
				err = fmt.Errorf("InitObject is not a Entity")
			}
		case ResourceTypeStatefulSet:
			if sts, ok := h.InitObject.(*appsv1.StatefulSet); ok {
				if reflect.DeepEqual(sts.Spec, obj.(*appsv1.StatefulSet).Spec) {
					return nil, nil
				}
				sts.Spec = *obj.(*appsv1.StatefulSet).Spec.DeepCopy()
			} else {
				err = fmt.Errorf("InitObject is not a StatefulSet")
			}
		case ResourceTypeDeployment:
			if deploy, ok := h.InitObject.(*appsv1.Deployment); ok {
				if reflect.DeepEqual(deploy.Spec, obj.(*appsv1.Deployment).Spec) {
					return nil, nil
				}
				deploy.Spec = *obj.(*appsv1.Deployment).Spec.DeepCopy()
			} else {
				err = fmt.Errorf("InitObject is not a Deployment")
			}
		case ResourceTypeReplicaSet:
			if rs, ok := h.InitObject.(*appsv1.ReplicaSet); ok {
				if reflect.DeepEqual(rs.Spec, obj.(*appsv1.ReplicaSet).Spec) {
					return nil, nil
				}
				rs.Spec = *obj.(*appsv1.ReplicaSet).Spec.DeepCopy()
			} else {
				err = fmt.Errorf("InitObject is not a ReplicaSet")
			}
		case ResourceTypePod:
			if pod, ok := h.InitObject.(*corev1.Pod); ok {
				if reflect.DeepEqual(pod.Spec, obj.(*corev1.Pod).Spec) {
					return nil, nil
				}
				pod.Spec = *obj.(*corev1.Pod).Spec.DeepCopy()
			} else {
				err = fmt.Errorf("InitObject is not a Pod")
			}
		case ResourceTypeNamespace:
			if ns, ok := h.InitObject.(*corev1.Namespace); ok {
				if reflect.DeepEqual(ns.Spec, obj.(*corev1.Namespace).Spec) {
					return nil, nil
				}
				ns.Spec = *obj.(*corev1.Namespace).Spec.DeepCopy()
			} else {
				err = fmt.Errorf("InitObject is not a Namespace")
			}
		case ResourceTypeIngress:
			if ing, ok := h.InitObject.(*networkingv1.Ingress); ok {
				if reflect.DeepEqual(ing.Spec, obj.(*networkingv1.Ingress).Spec) {
					return nil, nil
				}
				ing.Spec = *obj.(*networkingv1.Ingress).Spec.DeepCopy()
			} else {
				err = fmt.Errorf("InitObject is not an Ingress")
			}
		case ResourceTypeNetPol:
			if np, ok := h.InitObject.(*networkingv1.NetworkPolicy); ok {
				if reflect.DeepEqual(np.Spec, obj.(*networkingv1.NetworkPolicy).Spec) {
					return nil, nil
				}
				np.Spec = *obj.(*networkingv1.NetworkPolicy).Spec.DeepCopy()
			} else {
				err = fmt.Errorf("InitObject is not a NetworkPolicy")
			}
		// ... Add cases for other resource types (Certificate, CronJob, etc.) ...
		case ResourceTypeCronJob:
			if cj, ok := h.InitObject.(*batchv1.CronJob); ok {
				if reflect.DeepEqual(cj.Spec, obj.(*batchv1.CronJob).Spec) {
					return nil, nil
				}
				cj.Spec = *obj.(*batchv1.CronJob).Spec.DeepCopy()
			} else {
				err = fmt.Errorf("InitObject is not a CronJob")
			}
		case ResourceTypeService:
			if srv, ok := h.InitObject.(*corev1.Service); ok {
				if reflect.DeepEqual(srv.Spec, obj.(*corev1.Service).Spec) {
					return nil, nil
				}
				srv.Spec = *obj.(*corev1.Service).Spec.DeepCopy()
			} else {
				err = fmt.Errorf("InitObject is not a Service")
			}
		case ResourceTypeServiceAccount:
			// skip service account as it has no spec.
			return nil, nil
		case ResourceTypeRoleBinding:
			if rb, ok := h.InitObject.(*rbacv1.RoleBinding); ok {
				if reflect.DeepEqual(rb.RoleRef, obj.(*rbacv1.RoleBinding).RoleRef) {
					return nil, nil
				}
				rb.RoleRef = *obj.(*rbacv1.RoleBinding).RoleRef.DeepCopy()
			} else {
				err = fmt.Errorf("InitObject is not a RoleBinding")
			}
		case ResourceTypeCert:
			if crt, ok := h.InitObject.(*certmanager.Certificate); ok {
				if reflect.DeepEqual(crt.Spec, obj.(*certmanager.Certificate).Spec) {
					return nil, nil
				}
				crt.Spec = *obj.(*certmanager.Certificate).Spec.DeepCopy()
			} else {
				err = fmt.Errorf("InitObject is not a Service")
			}
		default:
			err = fmt.Errorf("unsupported resource type: %s", h.ResourceType)
		}
		if err != nil {
			return &Result{
				ResourceName: h.TargetName,
				Message:      fmt.Sprintf("Initial Object mismatch %s.", h.TargetName),
				Status:       "Failed",
				Reason:       "Initalize Object failed",
				ResourceType: h.ResourceType,
			}, err
		}
		// do not expecte version conflict, but if it does, retry on conflict
		if err := client.Update(ctx, h.InitObject); err != nil {
			if k8sErrors.IsConflict(err) {
				// restart the handle function
				return h.Handle(ctx, client, obj, f)
			}
			// Handle other errors
			return &Result{
				ResourceName: h.TargetName,
				Message:      fmt.Sprintf("Failed to update existing Object %s.", h.TargetName),
				Status:       "Failed",
				Reason:       "Update Object failed",
				ResourceType: h.ResourceType,
			}, err
		}
	}

	if h.NeedVerify {
		var err error
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			switch h.ResourceType {
			case ResourceTypeDeployment:
				err = h.verifyDeploymentReady(ctx, client)
			// Add cases for other resource types (StatefulSet, Pod, etc.) as needed
			case ResourceTypePod:
				err = h.verifyPodReady(ctx, client)
			case ResourceTypeReplicaSet:
				err = h.verifyReplicaSetReady(ctx, client)
			case ResourceTypeStatefulSet:
				err = h.verifyStatefulSetReady(ctx, client)
			default:
				err = fmt.Errorf("verification not implemented for resource type: %s", h.ResourceType)
			}
			if err == nil {
				goto final
			}
			log.FromContext(ctx).Info(fmt.Sprintf("Resource %s not ready, retrying...", h.TargetName), "error", err)
			time.Sleep(10 * time.Second)
		}

	}

final:
	return &Result{
		ResourceName: h.TargetName,
		Message:      fmt.Sprintf("Resource %s is ready.", h.TargetName),
		Status:       "Success",
		Reason:       "provision successfuly",
		ResourceType: h.ResourceType,
	}, nil
}

// Helper function to verify Deployment readiness
func (h *ClusterResourceHandler) verifyDeploymentReady(ctx context.Context, client k8sClient.Client) error {
	deployment := &appsv1.Deployment{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: h.TargetName}, deployment); err != nil {
		return fmt.Errorf("failed to get Deployment: %w", err)
	}

	// Get the latest condition
	conditions := deployment.Status.Conditions
	if len(conditions) == 0 {
		return fmt.Errorf("deployment has no conditions yet")
	}
	lastCondition := conditions[len(conditions)-1]

	if lastCondition.Type == appsv1.DeploymentAvailable && lastCondition.Status == corev1.ConditionTrue {
		return nil // Deployment is ready
	}

	return fmt.Errorf("deployment is not yet ready (reason: %s, message: %s)", lastCondition.Reason, lastCondition.Message)
}

// Helper function to verify Pod readiness
func (h *ClusterResourceHandler) verifyPodReady(ctx context.Context, client k8sClient.Client) error {
	pod := &corev1.Pod{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: h.TargetName}, pod); err != nil {
		return fmt.Errorf("failed to get Pod: %w", err)
	}

	// Get the latest condition
	conditions := pod.Status.Conditions
	if len(conditions) == 0 {
		return fmt.Errorf("pod has no conditions yet")
	}
	lastCondition := conditions[len(conditions)-1]

	// Assuming you are looking for a "Ready" condition for the Pod
	if lastCondition.Type == corev1.PodReady && lastCondition.Status == corev1.ConditionTrue {
		return nil // Pod is ready
	}

	return fmt.Errorf("pod is not yet ready (reason: %s, message: %s)", lastCondition.Reason, lastCondition.Message)
}

// Helper function to verify ReplicaSet readiness
func (h *ClusterResourceHandler) verifyReplicaSetReady(ctx context.Context, client k8sClient.Client) error {
	rs := &appsv1.ReplicaSet{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: h.TargetName}, rs); err != nil {
		return fmt.Errorf("failed to get ReplicaSet: %w", err)
	}

	// Check if all replicas are ready
	if rs.Status.ReadyReplicas == *rs.Spec.Replicas {
		return nil // ReplicaSet is ready
	}

	return fmt.Errorf("replicaset is not yet ready")
}

// Helper function to verify StatefulSet readiness
func (h *ClusterResourceHandler) verifyStatefulSetReady(ctx context.Context, client k8sClient.Client) error {
	sts := &appsv1.StatefulSet{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: h.Namespace, Name: h.TargetName}, sts); err != nil {
		return fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	// Check if all replicas are ready
	if sts.Status.ReadyReplicas == *sts.Spec.Replicas {
		return nil // StatefulSet is ready
	}

	return fmt.Errorf("statefulset is not yet ready")
}

// Implement the Handler interface for GenericResourceHandler.
var _ Handler[Result] = &ClusterResourceHandler{}

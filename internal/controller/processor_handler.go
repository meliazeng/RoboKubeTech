package controller

import (
	"context"
	"fmt"
	"time"

	robokubev1 "meliazeng/RoboKube/api/v1"

	"encoding/json"
	"github.com/go-logr/logr"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8sErr "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"meliazeng/RoboKube/internal/controller/handler"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	DefaultRequeueDelay = 10 * time.Second
	EntityRequeueDelay  = 60 * time.Second
)

// RkubeType represents an object with a Status.Conditions field
type RkubeType interface {
	client.Object
	GetConditions() []robokubev1.Condition
	SetConditions([]robokubev1.Condition)
	GetPhase() string
	SetPhase(string)
}

type (
	buildFn[T any]  func(*client.Object, func(client.Object)) (T, error)
	procFn[T any]   func(context.Context, client.Client, client.Object, func(client.Object)) (*T, error)
	updateFn[T any] func(*T) func(client.Object)
)

// regular resource provisoning
func Processor[T any](ctx context.Context, r client.Client, res client.Object, obj client.Object, buildFn buildFn[client.Object], f func(client.Object), procFn procFn[T], updateFn updateFn[T], log logr.Logger) (*T, error) {
	var err error

	if obj != nil {
		if f != nil {
			log.Info("Modifying existing object", "objectType", fmt.Sprintf("%T", obj))
			f(obj)
		}
	} else {
		if buildFn != nil {
			log.Info("Building new object", "resource", res.GetName())
			obj, err = buildFn(&res, f)
			if err != nil {
				return nil, err
			}
		} else {
			if f != nil {
				log.Info("Modifying original object", "resource", res.GetName())
				f(res)
			}
			obj = res
		}
	}

	// 3. Process the object (new or modified original)
	log.Info("Processing object", "objectType", fmt.Sprintf("%T", obj))
	result, err := procFn(ctx, r, obj, f)
	if err != nil {
		return nil, err
	}

	// 4. Update status if updateFn is provided
	if result != nil && updateFn != nil {
		log.Info("Updating status", "resource", res.GetName())
		if err := UpdateStatus(ctx, r, res, updateFn(result)); err != nil {
			return nil, fmt.Errorf("failed to update status: %w", err)
		}
	}
	log.Info("Object processed successfully", "objectType", fmt.Sprintf("%T", obj))
	return result, nil
}

func ErrorDelegateHandler(ctx context.Context, r client.Client, res client.Object, err error) (ctrl.Result, error) {
	defer func(err error) {
		if updateErr := UpdateStatus(ctx, r, res, updateStatusPhase(robokubev1.StatusFailed),
			setErrorCondition("Failed", "Errored", err.Error())); updateErr != nil {
			log.FromContext(ctx).Error(updateErr, "Failed to update resource to failed state")
		}
	}(err)

	switch {
	case k8sErr.IsNotFound(err):
		log.FromContext(ctx).Info("Resource could not be found, ignoring...")
		return ctrl.Result{}, nil

	case k8sErr.IsConflict(err):
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err

	}

	log.FromContext(ctx).Error(err, "unknown error, stop reconciliation")
	return ctrl.Result{}, nil
}

func UpdateStatus(ctx context.Context, r client.Client, res client.Object, updateFns ...func(res client.Object)) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.Get(ctx, types.NamespacedName{Name: res.GetName(), Namespace: res.GetNamespace()}, res); err != nil {
			return fmt.Errorf("cloud not retrieve the object from cluster, %w", err)
		}

		for _, f := range updateFns {
			f(res)
		}

		return r.Status().Update(ctx, res)
	})

	if err != nil {
		return fmt.Errorf("failed to update resource, %w", err)
	}

	return nil
}

func updateStatusPhase(phase string) func(res client.Object) {
	return func(res client.Object) {
		if typedObj, ok := res.(RkubeType); ok {
			typedObj.SetPhase(phase)
		}
	}
}

func setErrorCondition(health, reason, message string) func(res client.Object) {
	return func(res client.Object) {
		if typedObj, ok := res.(RkubeType); ok {
			typedObj.SetConditions(append(typedObj.GetConditions(), robokubev1.Condition{
				Status:             health,
				LastTransitionTime: v1.NewTime(time.Now()),
				Reason:             reason,
				Message:            message,
			}))
		}
	}
}

func setCondition(result *handler.Result) func(res client.Object) {
	return func(res client.Object) {
		if typedObj, ok := res.(RkubeType); ok {
			typedObj.SetConditions(append(typedObj.GetConditions(), robokubev1.Condition{
				Status:             result.Status,
				LastTransitionTime: v1.NewTime(time.Now()),
				Reason:             result.Reason,
				Message:            result.Message,
			}))
		}
	}
}

func SetStatusData(result *handler.Result) func(res client.Object) {
	return func(res client.Object) {
		if typedObj, ok := res.(*robokubev1.Entity); ok {
			var jsonData apiextensions.JSON
			if err := json.Unmarshal([]byte(result.Message), &jsonData); err != nil {
				// Handle error, perhaps log it
				typedObj.Status.Data = apiextensions.JSON{
					Raw: []byte(`{"error": "Failed to parse JSON data"}`),
				}
			} else {
				typedObj.Status.Data = jsonData
			}
			typedObj.Status.LastSyncTime = v1.NewTime(time.Now())
		}
	}
}

// handleFinalizer handles adding and removing finalizers for a given resource.
func handleFinalizer(ctx context.Context, r client.Client, res client.Object, finalizer string) (ctrl.Result, error) {

	if !controllerutil.ContainsFinalizer(res, finalizer) {
		controllerutil.AddFinalizer(res, finalizer)

		if err := r.Update(ctx, res); err != nil {
			return ErrorDelegateHandler(ctx, r, res, fmt.Errorf("failed to add finalizer, %w", err))
		}
	}

	if (res).GetDeletionTimestamp() != nil {
		if err := UpdateStatus(ctx, r, res, updateStatusPhase("Deleting")); err != nil {
			return ErrorDelegateHandler(ctx, r, res, fmt.Errorf("failed to set resource to Processing state, %w", err))
		}

		if _, err := handler.NewDeletionHandler[client.Object]().Handle(ctx, r, res, nil); err != nil {
			return ctrl.Result{}, err
		}

		if controllerutil.ContainsFinalizer(res, finalizer) {
			controllerutil.RemoveFinalizer(res, finalizer)
			if err := r.Update(ctx, res); err != nil {
				return ErrorDelegateHandler(ctx, r, res, fmt.Errorf("failed to remove finalizer, %w", err))
			}
			return ctrl.Result{}, nil
		}
	}

	return reconcile.Result{}, nil
}

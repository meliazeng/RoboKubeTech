package handler

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceType represents the type of Kubernetes resource.

// Define constants for supported resource types.

type ObjectParams struct {
	ResourceType ResourceType
	Action       string
	IsDelete     bool
}

// ClusterResourceHandler handles operations for a generic Kubernetes resource.
type ObjectHandler struct {
	ResourceType ResourceType
	Action       string
	IsDelete     bool
}

func NewObjectHandler(parent client.Object, paral *ObjectParams) *ObjectHandler {
	return &ObjectHandler{
		ResourceType: paral.ResourceType,
		Action:       paral.Action,
		IsDelete:     paral.IsDelete,
	}
}

// Handle processes the operation for the specified resource type.
func (h *ObjectHandler) Handle(ctx context.Context, client client.Client, obj client.Object, f func(client.Object)) (*Result, error) {
	var err error

	if h.IsDelete {
		if internalErr := client.Delete(ctx, obj); internalErr != nil {
			err = fmt.Errorf("failed to delete object: %w", internalErr)
		}
	} else {
		if internalErr := UpdateObj(ctx, client, obj, f); internalErr != nil {
			err = fmt.Errorf("failed to update object: %w", internalErr)
		}
	}

	if err != nil {
		return &Result{
			ResourceName: obj.GetName(),
			Message:      fmt.Sprintf("Resource %s is failed to modify.", obj.GetName()),
			Status:       "Failed",
			Reason:       h.Action,
			ResourceType: h.ResourceType,
		}, err
	} else {
		return &Result{
			ResourceName: obj.GetName(),
			Message:      fmt.Sprintf("Resource %s is successfully modify.", obj.GetName()),
			Status:       "Success",
			Reason:       h.Action,
			ResourceType: h.ResourceType,
		}, nil
	}
}

func UpdateObj(ctx context.Context, r client.Client, res client.Object, updateFns ...func(client.Object)) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.Get(ctx, types.NamespacedName{Name: res.GetName(), Namespace: res.GetNamespace()}, res); err != nil {
			return fmt.Errorf("cloud not retrieve the object from cluster, %w", err)
		}

		for _, f := range updateFns {
			if f != nil {
				f(res)
			}
		}
		return r.Update(ctx, res)
	})

	if err != nil {
		return fmt.Errorf("failed to update resource, %w", err)
	}
	return nil
}

// Implement the Handler interface for GenericResourceHandler.
var _ Handler[Result] = &ObjectHandler{}

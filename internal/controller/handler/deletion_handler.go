package handler

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Deletable interface {
	client.Object
	GetObjectsToDelete() ([]func(context.Context, client.Client) error, error)
}

type DeletionHandler[T any] struct {
}

func NewDeletionHandler[T any]() *DeletionHandler[T] {
	return &DeletionHandler[T]{}
}

func (d DeletionHandler[T]) Handle(ctx context.Context, r client.Client, obj client.Object, f func(client.Object)) (*T, error) {
	deletableObj, ok := obj.(Deletable)
	if !ok {
		// Handle the case where 'obj' is not a Deletable
		return nil, fmt.Errorf("object is not Deletable")
	}

	deleteFns, err := deletableObj.GetObjectsToDelete()
	if err != nil {
		return nil, fmt.Errorf("failed to get objects to delete: %w", err)
	}

	for _, deleteFn := range deleteFns {
		if err := deleteFn(ctx, r); err != nil {
			log.FromContext(ctx).Error(err, "Failed to delete resource")
			return nil, err
		}
	}

	log.FromContext(ctx).Info("Completed deletion of resources derived from " + obj.GetName())
	return nil, nil
}

func (d DeletionHandler[T]) DeleteK8sCert(ctx context.Context, r client.Client, res client.Object) error {
	// List Secrets with the label "mode: <res.GetName()>"
	secretList := &corev1.SecretList{}
	labelSelector := client.MatchingLabels{"model": res.GetName()}
	if err := r.List(ctx, secretList, labelSelector); err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	// Delete each Secret in the list
	for _, secret := range secretList.Items {
		log.FromContext(ctx).Info("Deleting secret", "secretName", secret.Name)
		if err := r.Delete(ctx, &secret); err != nil {
			return fmt.Errorf("failed to delete secret %s: %w", secret.Name, err)
		}
	}

	return nil
}

var _ Handler[Deletable] = &DeletionHandler[Deletable]{}

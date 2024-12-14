package handler

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler[T any] interface {
	Handle(context.Context, client.Client, client.Object, func(client.Object)) (*T, error)
}

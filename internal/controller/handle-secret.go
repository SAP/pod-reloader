/*
SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and pod-reloader contributors
SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/sap/pod-reloader/internal/reloader"
)

const secretHandlerName = "secret-handler"

type secretHandler struct {
	genericHandler
}

var _ reconcile.Reconciler = &secretHandler{}

func newSecretHandler(mgr ctrl.Manager) *secretHandler {
	return &secretHandler{
		genericHandler{
			client:     mgr.GetClient(),
			recorder:   mgr.GetEventRecorderFor(controllerName),
			annotation: reloader.AnnotationSecrets,
		},
	}
}

func setupSecretHandler(mgr ctrl.Manager) error {
	c, err := controller.New(secretHandlerName, mgr, controller.Options{Reconciler: newSecretHandler(mgr), MaxConcurrentReconciles: 5})
	if err != nil {
		return err
	}
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, &handler.TypedEnqueueRequestForObject[*corev1.Secret]{})); err != nil {
		return err
	}
	return nil
}

func (h *secretHandler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	if err := h.handle(ctx, "Secret", request.Namespace, request.Name); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

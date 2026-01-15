/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and pod-reloader contributors
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

const configMapHandlerName = "configmap-handler"

type configMapHandler struct {
	genericHandler
}

var _ reconcile.Reconciler = &configMapHandler{}

func newConfigMapHandler(mgr ctrl.Manager) *configMapHandler {
	return &configMapHandler{
		genericHandler{
			client:     mgr.GetClient(),
			recorder:   mgr.GetEventRecorderFor(controllerName),
			annotation: reloader.AnnotationConfigMaps,
		},
	}
}

func setupConfigMapHandler(mgr ctrl.Manager) error {
	c, err := controller.New(configMapHandlerName, mgr, controller.Options{Reconciler: newConfigMapHandler(mgr), MaxConcurrentReconciles: 5})
	if err != nil {
		return err
	}
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.ConfigMap{}, &handler.TypedEnqueueRequestForObject[*corev1.ConfigMap]{})); err != nil {
		return err
	}
	return nil
}

func (h *configMapHandler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	if err := h.handle(ctx, "ConfigMap", request.Namespace, request.Name); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

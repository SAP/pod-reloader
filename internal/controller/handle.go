/*
SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and pod-reloader contributors
SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sap/pod-reloader/internal/reloader"
)

type genericHandler struct {
	client     ctrlclient.Client
	recorder   record.EventRecorder
	annotation string
}

func (h *genericHandler) handle(ctx context.Context, kind string, namespace string, name string) error {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("running reconcile")

	objects := make([]ctrlclient.Object, 0)

	// add additional workload types here
	// TODO: should we allow to restrict workload listing by some selector (would make sense in the case the webhook uses selectors)

	deploymentList := appsv1.DeploymentList{}
	if err := h.client.List(ctx, &deploymentList, &ctrlclient.ListOptions{Namespace: namespace}); err != nil {
		return err
	}
	for i := 0; i < len(deploymentList.Items); i++ {
		objects = append(objects, &deploymentList.Items[i])
	}

	statefulSetList := appsv1.StatefulSetList{}
	if err := h.client.List(ctx, &statefulSetList, &ctrlclient.ListOptions{Namespace: namespace}); err != nil {
		return err
	}
	for i := 0; i < len(statefulSetList.Items); i++ {
		objects = append(objects, &statefulSetList.Items[i])
	}

	daemonSetList := appsv1.DaemonSetList{}
	if err := h.client.List(ctx, &daemonSetList, &ctrlclient.ListOptions{Namespace: namespace}); err != nil {
		return err
	}
	for i := 0; i < len(daemonSetList.Items); i++ {
		objects = append(objects, &daemonSetList.Items[i])
	}

	for _, object := range objects {
		annotations := object.GetAnnotations()
		if annotations[h.annotation] != "" && contains(strings.Split(annotations[h.annotation], ","), name) {
			log.Info("annotating object", "kind", object.GetObjectKind().GroupVersionKind(), "namespace", object.GetNamespace(), "name", object.GetName())
			hash, err := reloader.GenerateHashForObject(ctx, h.client, object)
			if err != nil {
				return err
			}
			annotations[reloader.AnnotationConfigHash] = hash
			object.SetAnnotations(annotations)
			if err := h.client.Update(ctx, object); err != nil {
				return err
			}
			h.recorder.Eventf(object, corev1.EventTypeNormal, "ConfigurationChanged", "Reload triggered due to change of referenced %s %s/%s", kind, namespace, name)
		}
	}

	return nil
}

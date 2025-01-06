/*
SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and pod-reloader contributors
SPDX-License-Identifier: Apache-2.0
*/

package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/sap/pod-reloader/internal/reloader"
)

type mutator struct {
	scheme  *runtime.Scheme
	client  ctrlclient.Client
	decoder admission.Decoder
}

func (m *mutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := ctrl.LoggerFrom(ctx)
	log = log.WithValues("kind", req.Kind, "namespace", req.Namespace, "name", req.Name)
	ctx = ctrl.LoggerInto(ctx, log)

	gvk := schema.GroupVersionKind{
		Group:   req.Kind.Group,
		Version: req.Kind.Version,
		Kind:    req.Kind.Kind,
	}

	object, err := m.scheme.New(gvk)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if err := m.decoder.Decode(req, object); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	switch req.Operation {
	case admissionv1.Create, admissionv1.Update:
		if err := m.handleCreateOrUpdate(ctx, object); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	default:
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("this admission webhook may only be called for create/update operations"))
	}
	rawObject, err := json.Marshal(object)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, rawObject)
}

func (m *mutator) handleCreateOrUpdate(ctx context.Context, object runtime.Object) error {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("running mutation webhook")

	var (
		objMeta     *metav1.ObjectMeta
		podTemplate *corev1.PodTemplateSpec
	)

	switch obj := object.(type) {
	// add additional workload types here
	case *appsv1.Deployment:
		objMeta = &obj.ObjectMeta
		podTemplate = &obj.Spec.Template
	case *appsv1.StatefulSet:
		objMeta = &obj.ObjectMeta
		podTemplate = &obj.Spec.Template
	case *appsv1.DaemonSet:
		objMeta = &obj.ObjectMeta
		podTemplate = &obj.Spec.Template
	default:
		return fmt.Errorf("webhook called with unsupported object kind: %s", object.GetObjectKind().GroupVersionKind())
	}

	if objMeta.Annotations[reloader.AnnotationConfigMaps] == "" && objMeta.Annotations[reloader.AnnotationSecrets] == "" {
		return nil
	}

	hash, err := reloader.GenerateHashForObject(ctx, m.client, objMeta)
	if err != nil {
		return err
	}

	if injectedHash, ok := objMeta.Annotations[reloader.AnnotationConfigHash]; ok {
		log.Info("got injected configuration hash (probably set by controller due to config map or secret change)")
		if injectedHash != hash {
			return fmt.Errorf("injected hash does not match calculated hash")
		}
		delete(objMeta.Annotations, reloader.AnnotationConfigHash)
	}

	currentHash := podTemplate.Annotations[reloader.AnnotationConfigHash]
	if currentHash == "" {
		log.Info("setting initial configuration hash")
	} else if hash != currentHash {
		log.Info("updating configuration hash")
	}

	if podTemplate.Annotations == nil {
		podTemplate.Annotations = make(map[string]string)
	}
	podTemplate.Annotations[reloader.AnnotationConfigHash] = hash

	return nil
}

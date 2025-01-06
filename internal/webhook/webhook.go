/*
SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and pod-reloader contributors
SPDX-License-Identifier: Apache-2.0
*/

package webhook

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func SetupMutatingWebhookWithManager(mgr ctrl.Manager) {
	scheme := mgr.GetScheme()
	client := mgr.GetClient()
	decoder := admission.NewDecoder(scheme)
	mgr.GetWebhookServer().Register("/mutate", &webhook.Admission{Handler: &mutator{scheme: scheme, client: client, decoder: decoder}})
}

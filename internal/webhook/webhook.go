/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and pod-reloader contributors
SPDX-License-Identifier: Apache-2.0
*/

package webhook

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func SetupMutatingWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/mutate", &webhook.Admission{Handler: &mutator{scheme: mgr.GetScheme(), client: mgr.GetClient()}})
}

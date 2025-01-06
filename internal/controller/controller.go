/*
SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and pod-reloader contributors
SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"
)

const controllerName = "pod-reloader"

func SetupControllerWithManager(mgr ctrl.Manager) error {
	if err := setupConfigMapHandler(mgr); err != nil {
		return err
	}
	if err := setupSecretHandler(mgr); err != nil {
		return err
	}
	return nil
}

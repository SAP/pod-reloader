//go:build tools
// +build tools

/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and pod-reloader contributors
SPDX-License-Identifier: Apache-2.0
*/

package tools

import (
	_ "sigs.k8s.io/controller-runtime/tools/setup-envtest"
)

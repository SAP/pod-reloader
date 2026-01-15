/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and pod-reloader contributors
SPDX-License-Identifier: Apache-2.0
*/

package controller

func contains[T comparable](s []T, x T) bool {
	for _, y := range s {
		if y == x {
			return true
		}
	}
	return false
}

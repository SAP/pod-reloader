/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and pod-reloader contributors
SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const CertManagerVersion = "v1.12.1"

// run command and print stdout/stderr
func run(ctx context.Context, name string, arg ...string) error {
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// create kind cluster
func createKindCluster(ctx context.Context, kind string, name string, kubeconfig string) (err error) {
	if kind == "" {
		return fmt.Errorf("no kind executable was found or provided")
	}
	if err := run(ctx, kind, "version"); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			deleteKindCluster(kind, name)
		}
	}()
	if err := run(ctx, kind, "create", "cluster", "--name", name, "--kubeconfig", kubeconfig, "--wait", "300s"); err != nil {
		return err
	}

	return nil
}

// delete kind cluster
func deleteKindCluster(kind string, name string) error {
	if kind == "" {
		return fmt.Errorf("no kind executable was found or provided")
	}
	if err := run(context.Background(), kind, "version"); err != nil {
		return err
	}
	if err := run(context.Background(), kind, "delete", "cluster", "--name", name); err != nil {
		return err
	}

	return nil
}

// assemble mutatingwebhookconfiguration descriptor
func buildMutatingWebhookConfiguration() *admissionv1.MutatingWebhookConfiguration {
	return &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mutate",
		},
		Webhooks: []admissionv1.MutatingWebhook{{
			Name:                    "mutate.test.local",
			AdmissionReviewVersions: []string{"v1"},
			ClientConfig: admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Path: &[]string{"/mutate"}[0],
				},
			},
			Rules: []admissionv1.RuleWithOperations{{
				Operations: []admissionv1.OperationType{
					admissionv1.Create,
					admissionv1.Update,
				},
				Rule: admissionv1.Rule{
					APIGroups:   []string{"apps"},
					APIVersions: []string{"v1"},
					Resources:   []string{"deployments", "statefulsets", "daemonsets"},
				},
			}},
			SideEffects: &[]admissionv1.SideEffectClass{admissionv1.SideEffectClassNone}[0],
		}},
	}
}

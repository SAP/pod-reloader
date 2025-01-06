/*
SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and pod-reloader contributors
SPDX-License-Identifier: Apache-2.0
*/

package reloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func GenerateHash(ctx context.Context, client ctrlclient.Client, namespace string, configMapNames []string, secretNames []string) (string, error) {
	s := ""
	for _, configMapName := range configMapNames {
		s += "configmap/" + namespace + "/" + configMapName + "/"
		configMap := corev1.ConfigMap{}
		err := client.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: configMapName}, &configMap)
		if err == nil {
			s += string(configMap.UID) + "." + configMap.ResourceVersion + "\n"
		} else if errors.IsNotFound(err) {
			s += "\n"
		} else {
			return "", err
		}
	}
	for _, secretName := range secretNames {
		s += "secret/" + namespace + "/" + secretName + "/"
		secret := corev1.Secret{}
		err := client.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: secretName}, &secret)
		if err == nil {
			s += string(secret.UID) + "." + secret.ResourceVersion + "\n"
		} else if errors.IsNotFound(err) {
			s += "\n"
		} else {
			return "", err
		}
	}
	return sha256sum(s), nil
}

func GenerateHashForObject(ctx context.Context, client ctrlclient.Client, object metav1.Object) (string, error) {
	var (
		configMapNames []string
		secretNames    []string
	)

	annotations := object.GetAnnotations()

	if annotations[AnnotationConfigMaps] != "" {
		configMapNames = strings.Split(annotations[AnnotationConfigMaps], ",")
	}
	if annotations[AnnotationSecrets] != "" {
		secretNames = strings.Split(annotations[AnnotationSecrets], ",")
	}

	return GenerateHash(ctx, client, object.GetNamespace(), configMapNames, secretNames)
}

func sha256sum(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

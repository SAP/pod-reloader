/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and pod-reloader contributors
SPDX-License-Identifier: Apache-2.0
*/

package reloader_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/sap/pod-reloader/internal/reloader"
)

var ctx context.Context
var cancel context.CancelFunc

func TestReloader(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Reloader Suite")
}

var _ = BeforeSuite(func() {
	By("setting up context")
	ctx, cancel = context.WithCancel(context.TODO())
})

var _ = AfterSuite(func() {
	By("cancelling context")
	cancel()
})

var _ = Describe("Test hash computation (low level)", func() {
	var scheme *runtime.Scheme
	var cli ctrlclient.Client
	var namespace string

	BeforeEach(func() {
		namespace = "test"

		By("populating scheme")
		scheme = runtime.NewScheme()
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))

		By("creating fake client")
		cli = fakeclient.NewClientBuilder().WithScheme(scheme).Build()
	})

	It("should work with zero configmaps, zero secrets", func() {
		hash, err := reloader.GenerateHash(ctx, cli, namespace, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		// expected plain hash
		GinkgoWriter.Printf("Expected plain hash:\n")
		// expected sha256 hash (taken from some external hash calculator)
		expectedHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
		Expect(hash).To(Equal(expectedHash))
	})

	It("should work with one configmap, zero secrets", func() {
		err := cli.Create(ctx, buildConfigMap(namespace, "test1", "key", "value"))
		Expect(err).NotTo(HaveOccurred())
		hash, err := reloader.GenerateHash(ctx, cli, namespace, []string{"test1"}, nil)
		Expect(err).NotTo(HaveOccurred())
		// expected plain hash
		GinkgoWriter.Printf("Expected plain hash:\nconfigmap/%s/%s/ConfigMap/%s/%s.%s\n", namespace, "test1", namespace, "test1", "1")
		// expected sha256 hash (taken from some external hash calculator)
		expectedHash := "efec662502e54e0608b8416d527adc60299ebc1e67b528d2b835735ddba509cc"
		Expect(hash).To(Equal(expectedHash))
	})

	It("should work with zero configmaps, one secrets", func() {
		err := cli.Create(ctx, buildSecret(namespace, "test2", "key", "value"))
		Expect(err).NotTo(HaveOccurred())
		hash, err := reloader.GenerateHash(ctx, cli, namespace, nil, []string{"test2"})
		Expect(err).NotTo(HaveOccurred())
		// expected plain hash
		GinkgoWriter.Printf("Expected plain hash:\nsecret/%s/%s/Secret/%s/%s.%s\n", namespace, "test2", namespace, "test2", "1")
		// expected sha256 hash (taken from some external hash calculator)
		expectedHash := "a324647825255bf77512e38391358b3b1a38405c8f5e0d30e75693524a37a587"
		Expect(hash).To(Equal(expectedHash))
	})

	It("should work with multiple configmaps and secrets, some existing, some not", func() {
		err := cli.Create(ctx, buildConfigMap(namespace, "test3", "key", "value"))
		Expect(err).NotTo(HaveOccurred())
		err = cli.Create(ctx, buildSecret(namespace, "test6", "key", "value"))
		Expect(err).NotTo(HaveOccurred())
		hash, err := reloader.GenerateHash(ctx, cli, namespace, []string{"test3", "test4"}, []string{"test5", "test6"})
		Expect(err).NotTo(HaveOccurred())
		// expected plain hash
		GinkgoWriter.Printf("Expected plain hash:\nconfigmap/%s/%s/ConfigMap/%s/%s.%s\nconfigmap/%s/%s/\nsecret/%s/%s/\nsecret/%s/%s/Secret/%s/%s.%s\n",
			namespace, "test3", namespace, "test3", "1",
			namespace, "test4",
			namespace, "test5",
			namespace, "test6", namespace, "test6", "1")
		// expected sha256 hash (taken from some external hash calculator)
		expectedHash := "2b5b0e3ced239db9b0121559f6eed4d2e62b699afb9a71ba7adb9fea8355f751"
		Expect(hash).To(Equal(expectedHash))
	})
})

var _ = Describe("Test hash computation (from object annotation)", func() {
	var scheme *runtime.Scheme
	var cli ctrlclient.Client
	var namespace string

	BeforeEach(func() {
		namespace = "test"

		By("populating scheme")
		scheme = runtime.NewScheme()
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))

		By("creating fake client")
		cli = fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(
			buildConfigMap(namespace, "test1", "key", "value"),
			buildConfigMap(namespace, "test2", "key", "value"),
			buildSecret(namespace, "test1", "key", "value"),
			buildSecret(namespace, "test2", "key", "value"),
		).Build()
	})

	It("should work with zero configmaps, zero secrets", func() {
		var configMapNames []string
		var secretNames []string
		deployment := buildDeployment(namespace, "test", configMapNames, secretNames)
		hash, err := reloader.GenerateHashForObject(ctx, cli, deployment)
		Expect(err).NotTo(HaveOccurred())
		expectedHash, err := reloader.GenerateHash(ctx, cli, namespace, configMapNames, secretNames)
		Expect(err).NotTo(HaveOccurred())
		Expect(hash).To(Equal(expectedHash))

	})

	It("should work with multiple configmaps and secrets, some existing, some not", func() {
		configMapNames := []string{"test1", "test2", "test3"}
		secretNames := []string{"test1", "test2", "test3"}
		deployment := buildDeployment(namespace, "test", configMapNames, secretNames)
		hash, err := reloader.GenerateHashForObject(ctx, cli, deployment)
		Expect(err).NotTo(HaveOccurred())
		expectedHash, err := reloader.GenerateHash(ctx, cli, namespace, configMapNames, secretNames)
		Expect(err).NotTo(HaveOccurred())
		Expect(hash).To(Equal(expectedHash))
	})
})

func buildConfigMap(namespace string, name string, key string, value string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			UID:       types.UID("ConfigMap/" + namespace + "/" + name),
		},
		Data: map[string]string{
			key: value,
		},
	}
}

func buildSecret(namespace string, name string, key string, value string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			UID:       types.UID("Secret/" + namespace + "/" + name),
		},
		Data: map[string][]byte{
			key: []byte(value),
		},
	}
}

func buildDeployment(namespace string, name string, configMapNames []string, secretNames []string) *appsv1.Deployment {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			UID:       types.UID("Deployment/" + namespace + "/" + name),
		},
		Spec: appsv1.DeploymentSpec{},
	}
	if len(configMapNames) > 0 || len(secretNames) > 0 {
		deployment.Annotations = make(map[string]string)
	}
	if len(configMapNames) > 0 {
		deployment.Annotations[reloader.AnnotationConfigMaps] = strings.Join(configMapNames, ",")
	}
	if len(secretNames) > 0 {
		deployment.Annotations[reloader.AnnotationSecrets] = strings.Join(secretNames, ",")
	}
	return deployment
}

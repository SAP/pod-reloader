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
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"github.com/sap/go-generics/slices"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/sap/pod-reloader/internal/controller"
	"github.com/sap/pod-reloader/internal/reloader"
	"github.com/sap/pod-reloader/internal/webhook"
)

var enabled bool
var kubeconfig string
var image string
var hostname string
var kind string

func init() {
	var err error

	enabled = os.Getenv("E2E_ENABLED") == "true"
	kubeconfig = os.Getenv("E2E_KUBECONFIG")
	image = os.Getenv("E2E_IMAGE")

	hostname = os.Getenv("E2E_HOSTNAME")
	if hostname == "" {
		hostname, err = os.Hostname()
		if err != nil {
			panic(err)
		}
	}
	hostname = strings.ToLower(hostname)

	kind = os.Getenv("E2E_KIND")
	if kind == "" {
		kind, err = exec.LookPath("kind")
		if err != nil {
			kind = ""
		}
	}
}

func TestOperator(t *testing.T) {
	if !enabled {
		t.Skip("Skipped because end-to-end tests are not enabled")
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "Operator")
}

var kindEnv string
var testEnv *envtest.Environment
var cfg *rest.Config
var scheme *runtime.Scheme
var cli ctrlclient.Client
var ctx context.Context
var cancel context.CancelFunc
var threads sync.WaitGroup
var tmpdir string
var namespace string

var _ = BeforeSuite(func() {
	var err error

	if kubeconfig == "" && kind == "" {
		Fail("No kubeconfig provided, and no kind executable was provided or found in the path")
	}

	By("initializing")
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.TODO())
	tmpdir, err = os.MkdirTemp("", "")
	Expect(err).NotTo(HaveOccurred())

	if kubeconfig == "" {
		By("bootstrapping kind cluster")
		kindEnv = fmt.Sprintf("kind-%s", filepath.Base(tmpdir))
		kubeconfig = fmt.Sprintf("%s/kubeconfig", tmpdir)
		err := createKindCluster(ctx, kind, kindEnv, kubeconfig)
		Expect(err).NotTo(HaveOccurred())
	}

	By("fetching rest config")
	cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}, nil).ClientConfig()
	Expect(err).NotTo(HaveOccurred())

	By("populating scheme")
	scheme = runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	By("initializing client")
	cli, err = ctrlclient.New(cfg, ctrlclient.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	if image == "" {
		By("bootstrapping test environment")
		testEnv = &envtest.Environment{
			UseExistingCluster: &[]bool{true}[0],
			Config:             cfg,
			WebhookInstallOptions: envtest.WebhookInstallOptions{
				LocalServingHost: hostname,
				MutatingWebhooks: []*admissionv1.MutatingWebhookConfiguration{
					buildMutatingWebhookConfiguration(),
				},
			},
		}
		_, err = testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		webhookInstallOptions := &testEnv.WebhookInstallOptions

		By("creating manager")
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme,
			WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
				Host:    webhookInstallOptions.LocalServingHost,
				Port:    webhookInstallOptions.LocalServingPort,
				CertDir: webhookInstallOptions.LocalServingCertDir,
			}),
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
			HealthProbeBindAddress: "0",
		})
		Expect(err).NotTo(HaveOccurred())

		err = controller.SetupControllerWithManager(mgr)
		Expect(err).NotTo(HaveOccurred())
		webhook.SetupMutatingWebhookWithManager(mgr)

		By("starting manager")
		threads.Add(1)
		go func() {
			defer threads.Done()
			defer GinkgoRecover()
			err := mgr.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
		}()

		By("waiting for operator to become ready")
		Eventually(func() error { return mgr.GetWebhookServer().StartedChecker()(nil) }, "10s", "100ms").Should(Succeed())
	} else {
		By("bootstrapping test environment")
		testEnv = &envtest.Environment{
			UseExistingCluster: &[]bool{true}[0],
		}
		_, err = testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		// TODO: deploy image, rbac, service, webhook
		panic("not yet implemented")
	}

	By("creating testing namespace")
	namespace = createNamespace()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	threads.Wait()
	if testEnv != nil {
		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	}
	if kindEnv != "" {
		err := deleteKindCluster(kind, kindEnv)
		Expect(err).NotTo(HaveOccurred())
	}
	err := os.RemoveAll(tmpdir)
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Validate reload", func() {
	var configMap *corev1.ConfigMap
	var bumpConfigMap func()
	var secret *corev1.Secret
	var bumpSecret func()

	BeforeEach(func() {
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace,
				GenerateName: "test-",
			},
			Data: map[string]string{
				"key": uuid.NewString(),
			},
		}
		err := cli.Create(ctx, configMap)
		Expect(err).NotTo(HaveOccurred())

		bumpConfigMap = func() {
			configMap.Data["key"] = uuid.NewString()
			err := cli.Update(ctx, configMap)
			Expect(err).NotTo(HaveOccurred())
		}

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    namespace,
				GenerateName: "test-",
			},
			Data: map[string][]byte{
				"key": []byte(uuid.NewString()),
			},
		}
		err = cli.Create(ctx, secret)
		Expect(err).NotTo(HaveOccurred())

		bumpSecret = func() {
			secret.Data["key"] = []byte(uuid.NewString())
			err := cli.Update(ctx, secret)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	var _ = Describe("Validate reload for deployments", func() {
		var deployment *appsv1.Deployment

		BeforeEach(func() {
			app := uuid.NewString()
			deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    namespace,
					GenerateName: "test-",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": app,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": app,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "dummy",
									Image: "registry.k8s.io/pause:3.7",
								},
							},
						},
					},
				},
			}
			err := cli.Create(ctx, deployment)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should restart deployment if configmap is added", func() {
			enableReloadOn(deployment, configMap)
			waitForReloadComplete(deployment, 10*time.Second)
			bumpConfigMap()
			waitForReloadComplete(deployment, 10*time.Second)
		})

		It("should restart deployment if secret is added", func() {
			enableReloadOn(deployment, secret)
			waitForReloadComplete(deployment, 10*time.Second)
			bumpSecret()
			waitForReloadComplete(deployment, 10*time.Second)
		})
	})

	var _ = Describe("Validate reload for statefulsets", func() {
		var statefulSet *appsv1.StatefulSet

		BeforeEach(func() {
			app := uuid.NewString()
			statefulSet = &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    namespace,
					GenerateName: "test-",
				},
				Spec: appsv1.StatefulSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": app,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": app,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "dummy",
									Image: "registry.k8s.io/pause:3.7",
								},
							},
						},
					},
				},
			}
			err := cli.Create(ctx, statefulSet)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should restart statefulset if configmap is added", func() {
			enableReloadOn(statefulSet, configMap)
			waitForReloadComplete(statefulSet, 10*time.Second)
			bumpConfigMap()
			waitForReloadComplete(statefulSet, 10*time.Second)
		})

		It("should restart statefulset if secret is added", func() {
			enableReloadOn(statefulSet, secret)
			waitForReloadComplete(statefulSet, 10*time.Second)
			bumpSecret()
			waitForReloadComplete(statefulSet, 10*time.Second)
		})
	})

	var _ = Describe("Validate reload for daemonsets", func() {
		var daemonSet *appsv1.DaemonSet

		BeforeEach(func() {
			app := uuid.NewString()
			daemonSet = &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:    namespace,
					GenerateName: "test-",
				},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": app,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": app,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "dummy",
									Image: "registry.k8s.io/pause:3.7",
								},
							},
						},
					},
				},
			}
			err := cli.Create(ctx, daemonSet)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should restart daemonset if configmap is added", func() {
			enableReloadOn(daemonSet, configMap)
			waitForReloadComplete(daemonSet, 10*time.Second)
			bumpConfigMap()
			waitForReloadComplete(daemonSet, 10*time.Second)
		})

		It("should restart daemonset if secret is added", func() {
			enableReloadOn(daemonSet, secret)
			waitForReloadComplete(daemonSet, 10*time.Second)
			bumpSecret()
			waitForReloadComplete(daemonSet, 10*time.Second)
		})
	})
})

func createNamespace() string {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"}}
	err := cli.Create(ctx, namespace)
	Expect(err).NotTo(HaveOccurred())
	return namespace.Name
}

func enableReloadOn(object ctrlclient.Object, triggerObject ctrlclient.Object) {
	oldObject := object.DeepCopyObject().(ctrlclient.Object)

	annotationKey := ""

	if object.GetNamespace() != triggerObject.GetNamespace() {
		panic("this should not happen")
	}

	switch triggerObject.(type) {
	case *corev1.ConfigMap:
		annotationKey = reloader.AnnotationConfigMaps
	case *corev1.Secret:
		annotationKey = reloader.AnnotationSecrets
	default:
		panic("this should not happen")
	}

	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	var triggerNames []string

	if annotationValue, ok := annotations[annotationKey]; ok && annotationValue != "" {
		triggerNames = strings.Split(annotationValue, ",")
	}

	if !slices.Contains(triggerNames, triggerObject.GetName()) {
		triggerNames = append(triggerNames, triggerObject.GetName())
	}

	annotations[annotationKey] = strings.Join(triggerNames, ",")
	object.SetAnnotations(annotations)

	err := cli.Patch(ctx, object, ctrlclient.MergeFrom(oldObject))
	Expect(err).NotTo(HaveOccurred())
}

func waitForReloadComplete(object ctrlclient.Object, timeout time.Duration) {
	Eventually(func() error {
		if err := cli.Get(ctx, types.NamespacedName{Namespace: object.GetNamespace(), Name: object.GetName()}, object); err != nil {
			return err
		}
		hash, err := reloader.GenerateHashForObject(ctx, cli, object)
		if err != nil {
			return err
		}
		var labelSelector *metav1.LabelSelector
		switch object := object.(type) {
		case *appsv1.Deployment:
			labelSelector = object.Spec.Selector
		case *appsv1.StatefulSet:
			labelSelector = object.Spec.Selector
		case *appsv1.DaemonSet:
			labelSelector = object.Spec.Selector
		default:
			panic("this should not happen")
		}
		selector, err := metav1.LabelSelectorAsSelector(labelSelector)
		if err != nil {
			return err
		}
		podList := &corev1.PodList{}
		err = cli.List(ctx, podList, ctrlclient.InNamespace(namespace), ctrlclient.MatchingLabelsSelector{Selector: selector})
		if err != nil {
			return err
		}
		if len(podList.Items) == 0 {
			return fmt.Errorf("no pods found - try again")
		}
		for _, pod := range podList.Items {
			if pod.Annotations[reloader.AnnotationConfigHash] != hash {
				return fmt.Errorf("pod has wrong config hash - try again")
			}
		}
		return nil
	}, timeout, "1s").Should(Succeed())
}

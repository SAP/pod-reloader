/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and pod-reloader contributors
SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"flag"
	"net"
	"os"
	"strconv"

	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/sap/pod-reloader/internal/controller"
	"github.com/sap/pod-reloader/internal/webhook"
)

const (
	LeaderElectionID = "pod-reloader.cs.sap.com"
)

const (
	inClusterNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var webhookAddr string
	var webhookCertDir string
	var enableLeaderElection bool
	var leaderElectionNamespace string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&webhookAddr, "webhook-bind-address", ":9443", "The address the webhook endpoint binds to.")
	flag.StringVar(&webhookCertDir, "webhook-tls-directory", "", "The directory containing tls server key and certificate, as tls.key and tls.crt; defaults to $TMPDIR/k8s-webhook-server/serving-certs")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "", "The namespace to use for the leader election lock; defaults to controller namespace when running in-cluster.")
	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	inCluster, inClusterNamespace, err := checkInCluster()
	if err != nil {
		setupLog.Error(err, "unable to check if running in cluster")
		os.Exit(1)
	}

	webhookHost, webhookPort, err := parseAddress(webhookAddr)
	if err != nil {
		setupLog.Error(err, "unable to parse webhook bind address")
		os.Exit(1)
	}

	if enableLeaderElection && leaderElectionNamespace == "" {
		if inCluster {
			leaderElectionNamespace = inClusterNamespace
		} else {
			setupLog.Error(nil, "missing command line parameter", "flag", "--leader-election-namespace")
			os.Exit(1)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                        scheme,
		LeaderElection:                enableLeaderElection,
		LeaderElectionNamespace:       leaderElectionNamespace,
		LeaderElectionID:              LeaderElectionID,
		LeaderElectionReleaseOnCancel: true,
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Host:    webhookHost,
			Port:    webhookPort,
			CertDir: webhookCertDir,
		}),
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := controller.SetupControllerWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to set up controller")
		os.Exit(1)
	}

	webhook.SetupMutatingWebhookWithManager(mgr)

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func parseAddress(address string) (string, int, error) {
	host, p, err := net.SplitHostPort(address)
	if err != nil {
		return "", -1, err
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return "", -1, err
	}
	return host, port, nil
}

func checkInCluster() (bool, string, error) {
	_, err := os.Stat(inClusterNamespacePath)
	if os.IsNotExist(err) {
		return false, "", nil
	} else if err != nil {
		return false, "", errors.Wrap(err, "error checking namespace file")
	}

	namespace, err := os.ReadFile(inClusterNamespacePath)
	if err != nil {
		return false, "", errors.Wrap(err, "error reading namespace file")
	}

	return true, string(namespace), nil
}

package main

import (
	"log"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	podwebhook "github.com/openshift-cnv/kubevirt-force-rw-disk-addon/pkg/webhook/pod"
)

func main() {
	ctrllog.SetLogger(zap.New())
	scheme := runtime.NewScheme()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    8443,
			CertDir: "/tmp/k8s-webhook-server/serving-certs",
		}),
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		HealthProbeBindAddress: ":8081",
	})
	if err != nil {
		log.Fatalf("failed to create manager: %v", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Fatalf("failed to set up health check: %v", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Fatalf("failed to set up ready check: %v", err)
	}

	hookServer := mgr.GetWebhookServer()
	hookServer.Register("/mutate-pod", &webhook.Admission{
		Handler: podwebhook.NewMutatingHandler(),
	})

	log.Println("starting force-rw-disk webhook server on port 8443")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Fatalf("webhook server exited with error: %v", err)
	}
}

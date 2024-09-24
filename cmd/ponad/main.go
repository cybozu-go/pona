package main

import (
	"flag"
	"log/slog"
	"net"
	"os"
	"time"

	ponav1beta1 "github.com/cybozu-go/pona/api/v1beta1"
	"github.com/cybozu-go/pona/internal/ponad"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type Config struct {
	metricsAddr string
	healthAddr  string
	socketPath  string
	egressPort  int
}

const defaultSocketPath = "/run/ponad.sock"

const (
	gracefulTimeout = 20 * time.Second
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ponav1beta1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var config Config

	flag.StringVar(&config.metricsAddr, "metrics-addr", ":9384", "bind address of metrics endpoint")
	flag.StringVar(&config.healthAddr, "health-addr", ":9385", "bind address of health/readiness probes")
	flag.StringVar(&config.socketPath, "socket", defaultSocketPath, "UNIX domain socket path")
	flag.IntVar(&config.egressPort, "egress-port", 5555, "UDP port number for egress NAT")

	flag.Parse()

	l := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger := logr.FromSlogHandler(l.Handler())
	slog.SetDefault(l)
	klog.SetLogger(logger)
	ctrl.SetLogger(logger)

	mgr, err := setupManager(config)
	if err != nil {
		setupLog.Error(err, "failed to setup manager")
		os.Exit(1)
	}

	if err := startPonad(config, mgr); err != nil {
		setupLog.Error(err, "failed to start ponad")
		os.Exit(1)
	}
}

func setupManager(config Config) (ctrl.Manager, error) {
	timeout := gracefulTimeout
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		Metrics: metricsserver.Options{
			BindAddress: config.metricsAddr,
		},
		GracefulShutdownTimeout: &timeout,
		HealthProbeBindAddress:  config.healthAddr,
	})
	if err != nil {
		return nil, err
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return nil, err
	}
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		return nil, err
	}
	return mgr, nil
}

func startPonad(config Config, mgr ctrl.Manager) error {
	l, err := net.Listen("unix", config.socketPath)
	if err != nil {
		return err
	}

	s := ponad.NewServer(l, mgr.GetAPIReader(), config.egressPort)
	if err := mgr.Add(s); err != nil {
		return err
	}

	ctx := ctrl.SetupSignalHandler()
	slog.Info("starting manager")

	return mgr.Start(ctx)
}

package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/cybozu-go/pona/internal/ponad"
	"github.com/cybozu-go/pona/pkg/cnirpc"
	"github.com/cybozu-go/pona/pkg/tunnel/fou"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"google.golang.org/grpc"
)

type Config struct {
	metricsAddr string
	healthAddr  string
	socketPath  string
	egressPort  int
}

const defaultSocketPath = "/run/ponad.sock"

// InterceptorLogger adapts slog logger to interceptor logger.
// This code is simple enough to be copied and not imported.
func InterceptorLogger(l *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}

func main() {
	var config Config

	flag.StringVar(&config.metricsAddr, "metrics-addr", ":9384", "bind address of metrics endpoint")
	flag.StringVar(&config.healthAddr, "health-addr", ":9385", "bind address of health/readiness probes")
	flag.StringVar(&config.socketPath, "socket", defaultSocketPath, "UNIX domain socket path")
	flag.IntVar(&config.egressPort, "egress-port", 5555, "UDP port number for egress NAT")

	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	fc, err := fou.NewFoUTunnelController(config.egressPort, ipv4, ipv6)
	if err != nil {
		logger.Error("failed to generate FoU Tunnel Controller", slog.Any("error", err))
		os.Exit(1)
	}
	if err := fc.Init(); err != nil {
		logger.Error("failed to initialize FoU Tunnel Controller", slog.Any("error", err))
		os.Exit(1)
	}

	s := ponad.NewServer(fc)
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(
		logging.UnaryServerInterceptor(InterceptorLogger(logger)),
	))
	cnirpc.RegisterCNIServer(server, s)
}

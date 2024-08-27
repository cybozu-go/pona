package main

import (
	"flag"
)

type Config struct {
	metricsAddr string
	healthAddr  string
	socketPath  string
	egressPort  int
}

const defaultSocketPath = "/run/ponad.sock"

func main() {
	var config Config

	flag.StringVar(&config.metricsAddr, "metrics-addr", ":9384", "bind address of metrics endpoint")
	flag.StringVar(&config.healthAddr, "health-addr", ":9385", "bind address of health/readiness probes")
	flag.StringVar(&config.socketPath, "socket", defaultSocketPath, "UNIX domain socket path")
	flag.IntVar(&config.egressPort, "egress-port", 5555, "UDP port number for egress NAT")

	flag.Parse()
}

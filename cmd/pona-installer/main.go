package main

import (
	"log/slog"
	"os"

	"github.com/caarlos0/env/v10"
	_ "github.com/joho/godotenv/autoload"
)

type Config struct {
	CniEtcDir string `env:"CNI_ETC_DIR" envDefault:"/host/etc/cni/net.d"`
	CniBinDir string `env:"CNI_BIN_DIR" envDefault:"/host/opt/cni/bin"`
	PonaPath  string `env:"CNI_PATH" envDefault:"/pona"`
}

func main() {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		slog.Error("failed to parse config", slog.Any("error", err))
		os.Exit(1)
	}

	if err := installPona(cfg.PonaPath, cfg.CniBinDir); err != nil {
		slog.Error("failed to install pona",
			slog.Any("error", err),
		)
	}
}

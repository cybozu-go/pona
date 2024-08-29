package main

import (
	"log/slog"
	"os"

	"github.com/caarlos0/env/v10"
	_ "github.com/joho/godotenv/autoload"
)

type Config struct {
	CniConfName string `env:"CNI_CONF_NAME,required"`
	CniEtcDir   string `env:"CNI_ETC_DIR" envDefault:"/host/etc/cni/net.d"`
	CniBinDir   string `env:"CNI_BIN_DIR" envDefault:"/host/opt/cni/bin"`
	PonaPath    string `env:"CNI_PATH" envDefault:"/pona"`
	CniNetConf  string `env:"CNI_NET_CONF,required"`

	// cniEtcDir := viper.GetString("CNI_ETC_DIR")
	// cniBinDir := viper.GetString("CNI_BIN_DIR")
	// coilPath := viper.GetString("COIL_PATH")
	// cniNetConf := viper.GetString("CNI_NETCONF")
	// cniNetConfFile := viper.GetString("CNI_NETCONF_FILE")
}

func main() {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		slog.Error("failed to parse config", slog.Any("error", err))
		os.Exit(1)
	}
}

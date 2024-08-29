package main

import (
	"github.com/caarlos0/env/v10"
	_ "github.com/joho/godotenv/autoload"
)

type Config struct {
	CniConfName string `env:"CNI_CONF_NAME,required"`
	// cniEtcDir := viper.GetString("CNI_ETC_DIR")
	// cniBinDir := viper.GetString("CNI_BIN_DIR")
	// coilPath := viper.GetString("COIL_PATH")
	// cniNetConf := viper.GetString("CNI_NETCONF")
	// cniNetConfFile := viper.GetString("CNI_NETCONF_FILE")
}

func main() {

}

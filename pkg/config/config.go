package config

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/spf13/viper"
)

const confName = "ptt"

var confPath string

//ReadConfig is used to read config file from flag input or environment, if no config found will set default setting
func ReadConfig() error {
	flag.StringVar(&confPath, "c", "", "Configuration file path.")
	flag.Parse()
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()        // read in environment variables that match
	viper.SetEnvPrefix("ezptt") // will be uppercased automatically
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	var (
		err     error
		content []byte
	)
	if confPath != "" {
		content, err = ioutil.ReadFile(confPath)
		if err != nil {
			return err
		}
		viper.ReadConfig(bytes.NewBuffer(content))
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName(confName)
		// If a config file is found, read it in.
		if err = viper.ReadInConfig(); err == nil {
			fmt.Println("Using config file:", viper.ConfigFileUsed())
		} else {
			//default setting
			fmt.Println("Using default value")
			SetDefaults()
			err = nil
		}
	}
	return err
}

func SetDefaults() {
	// log level:
	//  0: Debug
	//  1: Info
	//  2: Warn
	//  3: Error
	viper.SetDefault("LOG.LEVEL", 0)
	viper.SetDefault("CRON.SPEC", "0 5 3 * * *")
}

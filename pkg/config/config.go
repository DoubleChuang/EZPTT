package config

import (
	"fmt"

	"github.com/spf13/viper"
)

const confName = "ptt"

var confPath string

type Config struct {
	LogLevel int    `mapstructure:"LOG_LEVEL"`
	CronSpec string `mapstructure:"CRON_SPEC"`
}

//LoadConfig is used to read config file from flag input or environment, if no config found will set default setting
// environment > .env > default
func LoadConfig(path string) (config Config, err error) {
	SetDefaults()
	viper.AddConfigPath(path)
	// viper.SetConfigName("")
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.SetEnvPrefix("ezptt") // will be uppercased automatically
	viper.AutomaticEnv()        // read in environment variables that match

	if err = viper.ReadInConfig(); err != nil {
		fmt.Println("ReadInConfig:", err)
	}

	err = viper.Unmarshal(&config)
	fmt.Println("config:", config)
	return
}

func SetDefaults() {
	// log level:
	//  0: Debug
	//  1: Info
	//  2: Warn
	//  3: Error
	viper.SetDefault("LOG_LEVEL", 0)
	viper.SetDefault("CRON_SPEC", "0 5 3 * * *")
}

package log

import (
	"log"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger *zap.SugaredLogger

func InitLog() {
	// Configuring Zap
	//
	// https://pkg.go.dev/go.uber.org/zap#hdr-Frequently_Asked_Questions
	var (
		l   *zap.Logger
		err error
	)
	logLevel := viper.GetInt("LOG.LEVEL")

	if logLevel > 0 {
		l, err = zap.NewProduction()
	} else { //Debug mode
		config := zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		l, err = config.Build()
	}

	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer l.Sync() // flushes buffer, if any
	Logger = l.Sugar()
}

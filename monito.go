package main

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/tejzpr/monito/log"
	"github.com/tejzpr/monito/monitors"
)

func main() {
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv()
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath("./appconfig")
	viper.SetEnvPrefix("MONITO")
	err := viper.ReadInConfig()
	log.Logger()
	if err != nil {
		log.Fatalf("fatal error config file: %s \n", err)
	}
	log.SetLogLevel(viper.GetString("logLevel"))

	log.Info("Starting monito")

	monitor1, err := monitors.NewHTTPMonitor(
		"A Monitor",
		log.Logger(),
		2*time.Second,
		10*time.Second,
		1,
		0,
		&monitors.HTTPConfig{
			URL:                "http://localhost:8080/ping",
			Method:             "GET",
			ExpectedBody:       ".",
			ExpectedStatusCode: 200,
		},
	)
	if err != nil {
		log.Fatalf("fatal error creating monitor: %s \n", err)
	}
	monitor1.Run(context.Background())
}

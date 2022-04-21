package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/viper"
	"github.com/tejzpr/monito/appconfig"
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

	var monitorWG sync.WaitGroup

	configuredMonitors := make(map[string]monitors.Monitor, 0)

	monitorsConfig := viper.GetStringMap("monitors")
	for monitorName, monitorConfigs := range monitorsConfig {
		log.Info("Starting monitors for", monitorName)
		mConfigArray := (monitorConfigs).([]interface{})
		for _, monitorConfig := range mConfigArray {
			if configuredMonitors[monitorName] != nil {
				log.Fatal("Monitor", monitorName, "already configured")
				return
			}
			jsonBody, err := json.Marshal(monitorConfig)
			if err != nil {
				log.Errorf(err, "Error marshalling config for monitor %s", monitorName)
				return
			}
			var mConfig appconfig.HTTPConfig
			if err := json.Unmarshal(jsonBody, &mConfig); err != nil {
				log.Errorf(err, "Error unmarshalling config for monitor %s", monitorName)
				return
			}
			monitor, err := monitors.NewHTTPMonitor(
				mConfig.Name,
				mConfig.Interval.Duration,
				mConfig.Timeout.Duration,
				mConfig.MaxConcurrentRequests,
				mConfig.MaxRetries,
				&monitors.HTTP{
					URL:                mConfig.URL,
					Method:             mConfig.Method,
					ExpectedBody:       mConfig.ExpectedResponseBody,
					ExpectedStatusCode: mConfig.ExpectedStatusCode,
				},
				log.Logger(),
			)
			if err != nil {
				log.Fatalf("fatal error creating monitor: %s \n", err)
			}
			monitorWG.Add(1)
			go func() error {
				defer func() {
					log.Info("Stopped monitor", mConfig.Name)
					monitorWG.Done()
				}()
				return monitor.Run(context.Background())
			}()
			configuredMonitors[mConfig.Name] = monitor
		}
	}
	stopper := make(chan os.Signal)
	signal.Notify(stopper, syscall.SIGTERM)
	signal.Notify(stopper, syscall.SIGINT)

	go func() {
		<-stopper
		log.Info("Stopping monito...")
		for _, monitor := range configuredMonitors {
			monitor.Stop()
		}
	}()

	monitorWG.Wait()
	log.Info("Stopped monito")
}

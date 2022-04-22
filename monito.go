package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/viper"
	"github.com/tejzpr/monito/log"
	"github.com/tejzpr/monito/monitors"
	"github.com/tejzpr/monito/notifiers"
	"github.com/tejzpr/monito/utils"
)

func main() {
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv()
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath("./config")
	viper.SetEnvPrefix("MONITO")
	err := viper.ReadInConfig()
	log.Logger()
	if err != nil {
		log.Fatalf("fatal error config file: %s \n", err)
	}
	log.SetLogLevel(viper.GetString("logLevel"))

	log.Info("Starting monito")

	// Initialize the notifiers
	notifierConfig := viper.GetStringMap("notifiers")
	for notifierName, notifierConfigs := range notifierConfig {
		log.Info(notifierName)
		err := notifiers.RegisterNotifier(notifierName, notifierConfigs)
		if err != nil {
			log.Errorf(err, "Failed to register notifier : %s", notifierName)
			return
		}
	}
	// Initialize the monitors
	var monitorWG sync.WaitGroup

	configuredMonitors := make(map[string]monitors.Monitor, 0)

	monitorsConfig := viper.GetStringMap("monitors")
	for monitorName, monitorConfigs := range monitorsConfig {
		log.Info("Starting monitors for: ", monitorName)
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
			var mConfig utils.HTTPConfig
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
				mConfig.NotifyRateLimit.Duration,
				func(err error) {
					log.Errorf(err, "Monitor %s failed", monitorName)
				},
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
					log.Info("Stopped monitor: ", mConfig.Name)
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
		closeMonitorsChan := make(chan struct{})

		go func() {
			for _, monitor := range configuredMonitors {
				monitor.Stop()
			}
		}()

		timer := time.NewTimer(10 * time.Second)
		select {
		case <-timer.C:
			log.Info("Timed out waiting for monitors to stop")
			log.Info("Exiting.")
			os.Exit(1)
		case <-closeMonitorsChan:
			log.Info("Monitors stopped")
		}

	}()

	monitorWG.Wait()
	log.Info("Exiting.")
}

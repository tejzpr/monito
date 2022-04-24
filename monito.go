package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"github.com/tejzpr/monito/log"

	// Initialize the monitors
	"github.com/tejzpr/monito/monitors"
	_ "github.com/tejzpr/monito/monitors/http"
	_ "github.com/tejzpr/monito/monitors/port"

	// Initialize the notifiers
	"github.com/tejzpr/monito/notifiers"
	_ "github.com/tejzpr/monito/notifiers/smtp"
	_ "github.com/tejzpr/monito/notifiers/webex"
	// Utils
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
	notifierConfigs := viper.GetStringMap("notifiers")
	log.Info("Initializing notifiers")
	for notifierName, notifierConfig := range notifierConfigs {
		jsonBody, err := json.Marshal(notifierConfig)
		if err != nil {
			log.Errorf(err, "Error marshalling config for notifier: %s", notifierName)
			return
		}
		_, err = notifiers.InitNotifier(notifierName, jsonBody)
		if err != nil {
			if err.Error() != "disabled" {
				log.Errorf(err, "Failed to register notifier: %s", notifierName)
				return
			}
		}
		log.Infof("Registered notifier: %s", notifierName)
	}
	log.Info("Notifiers initialized")
	// Initialize the monitors
	var monitorWG sync.WaitGroup

	configuredMonitors := make(map[string]monitors.Monitor, 0)

	monitorsConfig := viper.GetStringMap("monitors")
	for monitorName, monitorConfigs := range monitorsConfig {
		log.Info("Starting monitors for: ", monitorName)
		if !monitors.CheckIfMonitorRegistered(monitorName) {
			log.Errorf(fmt.Errorf("Monitor is not registered: %s", monitorName), "Monitor is not registered: %s", monitorName)
			continue
		}
		mConfigArray := (monitorConfigs).([]interface{})
		for _, monitorConfig := range mConfigArray {
			jsonBody, err := json.Marshal(monitorConfig)
			if err != nil {
				log.Errorf(err, "Error marshalling config for monitor: %s", monitorName)
				return
			}
			func(monitorConfig interface{}, jsonBody []byte) {
				mNotificationObj := (monitorConfig).(map[string]interface{})
				notifyDetails := mNotificationObj["notifyDetails"]
				notifiersObj := notifyDetails.(map[string]interface{})

				monitor, err := monitors.GetMonitor(monitorName, jsonBody, monitors.NotificationHandler(func(m monitors.Monitor, monitorerr error) {
					for _, notifyKey := range notifiers.GetRegisteredNotifierNames() {
						ntObj := notifiersObj[notifyKey]

						jBytes, err := json.Marshal(ntObj)
						if err != nil {
							log.Errorf(err, "Error marshalling config for notifier: %s", notifyKey)
							return
						}

						nt := notifiers.GetNotifier(notifyKey)
						if nt == nil {
							log.Errorf(err, "Failed to get smtp notifier for monitor: %s", m.Name())
							continue
						}
						if monitorerr != nil {
							subject := fmt.Sprintf("Failure in monitor : %s", m.Name())
							message := m.GetErrorNotificationBody(monitorerr)
							nt.Notify(subject, message, jBytes)
						} else {
							subject := fmt.Sprintf("Recovered : %s", m.Name())
							message := m.GetRecoveryNotificationBody()
							nt.Notify(subject, message, jBytes)
						}
					}
					log.Debugf("Failure in monitor : %s", m.Name())
				}), log.Logger(), viper.GetBool("metrics.prometheus.enable"))

				if err != nil {
					log.Fatalf("fatal error creating monitor: %s \n", err)
				}
				monitorWG.Add(1)
				go func() error {
					defer func() {
						log.Info("Stopped monitor: ", monitor.Name().String())
						monitorWG.Done()
					}()
					if !monitor.Enabled() {
						log.Info("Monitor is disabled: ", monitor.Name().String())
						return nil
					}
					log.Info("Running monitor: ", monitor.Name().String())
					err := monitor.Run(context.Background())
					if err != nil {
						log.Errorf(err, "Failed to run monitor: %s", monitor.Name())
						return err
					}
					return nil
				}()
				configuredMonitors[monitor.Name().String()] = monitor
			}(monitorConfig, jsonBody)
		}
	}
	stopper := make(chan os.Signal)
	signal.Notify(stopper, syscall.SIGTERM)
	signal.Notify(stopper, syscall.SIGINT)
	closeMonitorsChan := make(chan struct{})

	// Setup Metrics
	metricsPort := 8430
	isMetricsEnabled := false
	if viper.GetInt("metrics.prometheus.port") > 0 {
		metricsPort = viper.GetInt("metrics.prometheus.port")
	}
	metricsServerString := fmt.Sprintf("127.0.0.1:%d", metricsPort)
	if viper.GetBool("metrics.prometheus.enable") {
		isMetricsEnabled = true
		if !viper.GetBool("metrics.prometheus.enableGoCollector") {
			prometheus.Unregister(collectors.NewGoCollector())
		}
		http.Handle("/metrics", promhttp.Handler())
		log.Info("Metrics enabled on port: ", metricsPort)
	}
	if isMetricsEnabled {
		go func() {
			http.ListenAndServe(metricsServerString, nil)
		}()
	}

	// End Metrics Setup

	go func() {
		<-stopper
		log.Info("Stopping monito...")

		go func() {
			for _, monitor := range configuredMonitors {
				if monitor.Enabled() {
					monitor.Stop()
				}
			}
			notifiers.StopAll()
			closeMonitorsChan <- struct{}{}
		}()

		timer := time.NewTimer(10 * time.Second)
		select {
		case <-timer.C:
			log.Info("Timed out waiting for monitors to stop")
			closeMonitorsChan <- struct{}{}
		}

	}()

	go func() {
		monitorWG.Wait()
		log.Info("No more monitors to run")
		stopper <- syscall.SIGTERM
	}()
	<-closeMonitorsChan
	log.Info("Monitors stopped")
	log.Info("Exiting.")
}

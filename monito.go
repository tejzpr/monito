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
	log.Info("Initializing notifiers")
	for notifierName, notifierConfigs := range notifierConfig {
		err := notifiers.RegisterNotifier(notifierName, notifierConfigs)
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
		mConfigArray := (monitorConfigs).([]interface{})
		for _, monitorConfig := range mConfigArray {
			jsonBody, err := json.Marshal(monitorConfig)
			if err != nil {
				log.Errorf(err, "Error marshalling config for monitor: %s", monitorName)
				return
			}
			var mConfig utils.HTTPConfig
			if err := json.Unmarshal(jsonBody, &mConfig); err != nil {
				log.Errorf(err, "Error unmarshalling config for monitor: %s", monitorName)
				return
			}
			if mConfig.Name.String() == "" {
				log.Fatal("Monitor name is empty")
			}
			if configuredMonitors[mConfig.Name.String()] != nil {
				log.Fatal("Monitor already configured: ", monitorName)
			}

			monitor, err := monitors.NewHTTPMonitor(
				mConfig.Name,
				mConfig.Interval.Duration,
				mConfig.Timeout.Duration,
				mConfig.MaxConcurrentRequests,
				mConfig.MaxRetries,
				mConfig.NotifyRateLimit.Duration,
				func(state *monitors.State, monitorerr error) {
					now := time.Now()
					loc, _ := time.LoadLocation("UTC")
					if len(mConfig.NotifyDetails.SMTP.To) > 0 {
						nt, err := notifiers.GetNotifier("smtp")
						if err != nil {
							log.Errorf(err, "Failed to get smtp notifier for monitor: %s", mConfig.Name)
							return
						}
						var mailObj notifiers.Mail
						if monitorerr != nil {
							mailObj = notifiers.Mail{
								To:      mConfig.NotifyDetails.SMTP.To,
								Cc:      mConfig.NotifyDetails.SMTP.Cc,
								Bcc:     mConfig.NotifyDetails.SMTP.Bcc,
								Subject: fmt.Sprintf("Failure in monitor : %s", mConfig.Name),
								Body: fmt.Sprintf("Failure in monitor [%s]: %s \nType: %s\nFailed URL: %s\nAlerted On: %s\nNext Possible Alert In: %s",
									mConfig.Name,
									monitorerr.Error(),
									monitorName,
									mConfig.URL,
									now.In(loc).Format(time.RFC1123),
									mConfig.NotifyRateLimit.Duration.String()),
							}
						} else if state.Current == monitors.StateStatusOK && state.Previous == monitors.StateStatusError {
							mailObj = notifiers.Mail{
								To:      mConfig.NotifyDetails.SMTP.To,
								Cc:      mConfig.NotifyDetails.SMTP.Cc,
								Bcc:     mConfig.NotifyDetails.SMTP.Bcc,
								Subject: fmt.Sprintf("Recovered : %s", mConfig.Name),
								Body: fmt.Sprintf("Recovered in monitor [%s]: %s \nType: %s\nRecovered URL: %s\nRecovered On: %s",
									mConfig.Name,
									state.Current,
									monitorName,
									mConfig.URL,
									state.StateChangeTime.In(loc).Format(time.RFC1123)),
							}
						}
						err = nt.Notify(mailObj)
						if err != nil {
							log.Errorf(err, "Failed to SMTP notify monitor: %s", mConfig.Name)
							return
						}
					}

					if len(mConfig.NotifyDetails.Webex.RoomID) > 0 {
						nt, err := notifiers.GetNotifier("webex")
						if err != nil {
							log.Errorf(err, "Failed to get webex notifier for monitor: %s", mConfig.Name)
							return
						}
						if monitorerr != nil {
							err = nt.Notify(fmt.Sprintf("Failure in monitor [%s]: %s \nType: %s\nFailed URL: %s\nAlerted On: %s\nNext Possible Alert In: %s",
								mConfig.Name,
								monitorerr.Error(),
								monitorName,
								mConfig.URL,
								now.In(loc).Format(time.RFC1123),
								mConfig.NotifyRateLimit.Duration.String()),
								mConfig.NotifyDetails.Webex.RoomID)
						} else if state.Current == monitors.StateStatusOK && state.Previous == monitors.StateStatusError {
							err = nt.Notify(fmt.Sprintf("Recovered in monitor [%s]: %s \nType: %s\nRecovered URL: %s\nRecovered On: %s",
								mConfig.Name,
								state.Current,
								monitorName,
								mConfig.URL,
								state.StateChangeTime.In(loc).Format(time.RFC1123)),
								mConfig.NotifyDetails.Webex.RoomID)
						}
						if err != nil {
							log.Errorf(err, "Failed to Webex notify monitor: %s", mConfig.Name)
							return
						}
					}

					log.Debugf("Failure in monitor : %s", mConfig.Name)
				},
				&monitors.HTTP{
					URL:                mConfig.URL,
					Method:             mConfig.Method,
					ExpectedBody:       mConfig.ExpectedResponseBody,
					ExpectedStatusCode: mConfig.ExpectedStatusCode,
				},
				viper.GetBool("metrics.prometheus.enable"),
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
			configuredMonitors[mConfig.Name.String()] = monitor
		}
	}
	stopper := make(chan os.Signal)
	signal.Notify(stopper, syscall.SIGTERM)
	signal.Notify(stopper, syscall.SIGINT)
	closeMonitorsChan := make(chan struct{})
	go func() {
		<-stopper
		log.Info("Stopping monito...")

		go func() {
			for _, monitor := range configuredMonitors {
				monitor.Stop()
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

	monitorWG.Wait()
	<-closeMonitorsChan
	log.Info("Monitors stopped")
	log.Info("Exiting.")
}

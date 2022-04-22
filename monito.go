package main

import (
	"context"
	"encoding/json"
	"fmt"
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
			if configuredMonitors[monitorName] != nil {
				log.Fatal("Monitor already configured: ", monitorName)
				return
			}
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
			monitor, err := monitors.NewHTTPMonitor(
				mConfig.Name,
				mConfig.Interval.Duration,
				mConfig.Timeout.Duration,
				mConfig.MaxConcurrentRequests,
				mConfig.MaxRetries,
				mConfig.NotifyRateLimit.Duration,
				func(monitorerr error) {
					if len(mConfig.NotifyDetails.SMTP.To) > 0 {
						nt, err := notifiers.GetNotifier("smtp")
						if err != nil {
							log.Errorf(err, "Failed to get smtp notifier for monitor: %s", mConfig.Name)
							return
						}
						mailObj := notifiers.Mail{
							To:      mConfig.NotifyDetails.SMTP.To,
							Cc:      mConfig.NotifyDetails.SMTP.Cc,
							Bcc:     mConfig.NotifyDetails.SMTP.Bcc,
							Subject: fmt.Sprintf("Failure in monitor : %s", mConfig.Name),
							Body:    fmt.Sprintf("Failure in monitor [%s]: %s \nType: %s\nFailed URL: %s\nNext Alert In: %s", mConfig.Name, monitorerr.Error(), monitorName, mConfig.URL, mConfig.NotifyRateLimit.Duration.String()),
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
						err = nt.Notify(fmt.Sprintf("Failure in monitor [%s]: %s \nType: %s\nFailed URL: %s\nNext Alert In: %s", mConfig.Name, monitorerr.Error(), monitorName, mConfig.URL, mConfig.NotifyRateLimit.Duration.String()), mConfig.NotifyDetails.Webex.RoomID)
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

	monitorWG.Wait()
	<-closeMonitorsChan
	log.Info("Monitors stopped")
	log.Info("Exiting.")
}

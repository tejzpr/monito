package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"

	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/random"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"github.com/tejzpr/monito/log"
	"github.com/tejzpr/monito/utils"
	"github.com/tejzpr/monito/utils/pprof"
	"github.com/tejzpr/monito/utils/templates"
	"github.com/ziflex/lecho/v3"

	// Initialize the monitors
	"github.com/tejzpr/monito/monitors"
	_ "github.com/tejzpr/monito/monitors/http"
	_ "github.com/tejzpr/monito/monitors/port"

	// Initialize the notifiers
	"github.com/tejzpr/monito/notifiers"
	_ "github.com/tejzpr/monito/notifiers/smtp"
	_ "github.com/tejzpr/monito/notifiers/webex"
	_ "github.com/tejzpr/monito/notifiers/webhook"
	// Utils
)

//go:embed public
var public embed.FS

func main() {
	isLive := len(os.Args) > 1 && os.Args[1] == "live"

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
		_, err = notifiers.InitNotifier(notifiers.NotifierName(notifierName), jsonBody)
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
			jitter := len(mConfigArray)
			if jitter > 10 {
				jitter = 10
			}
			func(monitorConfig interface{}, jitter int, jsonBody []byte) {
				mNotificationObj := (monitorConfig).(map[string]interface{})
				notifyDetails := mNotificationObj["notifyDetails"]
				notifiersObj := notifyDetails.(map[string]interface{})

				monitor, err := monitors.GetMonitor(monitorName, jsonBody, log.Logger(), jitter, viper.GetBool("metrics.prometheus.enable"))
				if err != nil {
					log.Fatalf("fatal error creating monitor: %s \n", err)
				}

				monitorWG.Add(1)
				go func(m monitors.Monitor) error {
					defer func() {
						log.Info("Stopped monitor: ", monitor.GetName().String())
						monitorWG.Done()
					}()
					if !monitor.IsEnabled() {
						log.Info("Monitor is disabled: ", monitor.GetName().String())
						return nil
					}
					log.Info("Initializing monitor: ", monitor.GetName().String())
					err = m.Init()
					if err != nil {
						log.Errorf(err, "Failed to initialize monitor: %s", monitor.GetName().String())
						return err
					}

					m.GetState().Subscribe(m.GetName().String(), func(state *monitors.State) {
						log.Debug("Monitor state changed: ", monitor.GetName().String())
						if m.IsEnabled() {
							for _, notifyKey := range notifiers.GetRegisteredNotifierNames() {
								ntObj := notifiersObj[notifyKey.String()]

								jBytes, err := json.Marshal(ntObj)
								if err != nil {
									log.Errorf(err, "Error marshalling config for notifier: %s", notifyKey)
									return
								}

								if state.IsPreviousStateAFinalState() {
									nt := notifiers.GetNotifier(notifyKey)
									if nt == nil {
										log.Errorf(err, "Failed to get smtp notifier for monitor: %s", m.GetName())
										continue
									}

									nBody := m.GetNotificationBody(state)
									nt.Notify(nBody, jBytes)
								}
							}
						}
					})

					log.Info("Running monitor: ", monitor.GetName().String())
					err := monitor.Run(context.Background())
					if err != nil {
						log.Errorf(err, "Failed to run monitor: %s", monitor.GetName())
						return err
					}
					return nil
				}(monitor)

				configuredMonitors[monitor.GetName().String()] = monitor
			}(monitorConfig, jitter, jsonBody)
		}
	}

	stopper := make(chan os.Signal)
	signal.Notify(stopper, syscall.SIGTERM)
	signal.Notify(stopper, syscall.SIGINT)
	closeMonitorsChan := make(chan struct{})

	// Setup Metrics
	metricsPort := 8430
	metricsHost := "localhost"
	isMetricsEnabled := false
	if viper.GetInt("metrics.port") > 0 {
		metricsPort = viper.GetInt("metrics.port")
	}

	if viper.GetString("metrics.host") != "" {
		metricsHost = viper.GetString("metrics.host")
	}

	webApp := echo.New()
	webApp.HideBanner = true

	webTemplates := &templates.Template{}
	if isLive {
		webTemplates.SetTemplates(template.Must(template.ParseGlob("public/views/*.html")))
	} else {
		webTemplates.SetTemplates(template.Must(template.ParseFS(public, "public/views/*.html")))
	}
	webApp.Renderer = webTemplates

	webApp.Use(middleware.Recover())
	lechoLogger := lecho.From(log.ZLogger())
	webApp.Logger = lechoLogger
	webApp.Use(middleware.RequestID())
	webApp.Use(lecho.Middleware(lecho.Config{
		Logger: lechoLogger,
	}))

	if len(viper.GetString("metrics.cors")) > 0 {
		webApp.Use(middleware.CORS())
	}

	if viper.GetBool("metrics.enableGzip") {
		webApp.Use(middleware.GzipWithConfig(middleware.GzipConfig{
			Skipper: func(c echo.Context) bool {
				if c.Request().Header.Get("x-no-compression") != "" {
					return true
				}
				if strings.HasSuffix(c.Path(), "/ws") {
					return true
				}
				if strings.HasPrefix(c.Path(), "/metrics") {
					return true
				}
				if strings.HasPrefix(c.Path(), "/debug") {
					return true
				}
				return false
			},
			Level: 5,
		}))
	}
	root := webApp.Group("")
	root.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "pong")
	})
	metricsServerString := fmt.Sprintf("%s:%d", metricsHost, metricsPort)
	if viper.GetBool("metrics.pprof.enable") {
		isMetricsEnabled = true
		pprof.GetPPROF(root)
		log.Info("PProf enabled")
	}
	if viper.GetBool("metrics.prometheus.enable") {
		isMetricsEnabled = true
		if !viper.GetBool("metrics.prometheus.enableGoCollector") {
			prometheus.Unregister(collectors.NewGoCollector())
		}
		root.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
		log.Info("Prometheus metrics enabled on /metrics")
	}
	if viper.GetBool("metrics.monitostatus.enable") {
		isMetricsEnabled = true
		type monitorDetail struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Group       string `json:"group"`
		}
		type monitorWSStatus struct {
			Status    monitors.StateStatus `json:"status"`
			TimeStamp time.Time            `json:"timestamp"`
			Group     string               `json:"group"`
		}
		type monitorStatus struct {
			Name      string               `json:"name"`
			Status    monitors.StateStatus `json:"status"`
			TimeStamp time.Time            `json:"timestamp"`
			Group     string               `json:"group"`
		}
		root.GET("/api/monitors", func(c echo.Context) error {
			monitors := make(map[string][]*monitorDetail)

			monitorList := make([]*monitorDetail, 0)
			for _, monitor := range configuredMonitors {
				if monitor.IsEnabled() {
					monitorList = append(monitorList, &monitorDetail{
						Name:        monitor.GetName().String(),
						Description: monitor.GetDescription(),
						Group:       monitor.GetGroup(),
					})
				}
			}
			monitors["monitors"] = monitorList
			return c.JSON(http.StatusOK, monitors)
		})
		root.GET("/api/monitors/status", func(c echo.Context) error {
			monitors := make(map[string][]*monitorStatus)

			monitorList := make([]*monitorStatus, 0)
			for _, monitor := range configuredMonitors {
				if monitor.IsEnabled() {
					monitorList = append(monitorList, &monitorStatus{
						Name:      monitor.GetName().String(),
						Status:    monitor.GetState().GetCurrent(),
						TimeStamp: monitor.GetState().GetStateChangeTime(),
						Group:     monitor.GetGroup(),
					})
				}
			}
			monitors["monitors"] = monitorList
			return c.JSON(http.StatusOK, monitors)
		})
		root.GET("/api/monitors/ws", func(c echo.Context) error {
			conn, _, _, err := ws.UpgradeHTTP(c.Request(), c.Response())
			var stopOnce sync.Once
			if err != nil {
				log.Error(err, "WS: Failed to upgrade websocket connection")
				return err
			}
			rid := random.String(32)

			defer func() {
				log.Debug("WS: CLEANUP connection: ", rid)
				conn.Close()
			}()

			var wg sync.WaitGroup
			wg.Add(1)
			stop := func() {
				stopOnce.Do(func() {
					for _, monitor := range configuredMonitors {
						monitor.GetState().UnSubscribe(rid)
					}
					wg.Done()
				})
			}

			key, op, err := wsutil.ReadClientData(conn)
			if err != nil {
				log.Error(err, "WS: Failed to read client data")
				stop()
				return err
			}

			monitorsStatus := make(map[string]interface{})
			if len(key) > 0 && strings.ToLower(string(key)) == "all" {
				for monitorName, monitor := range configuredMonitors {
					if monitor.IsEnabled() {
						monitorsStatus[monitorName] = &monitorWSStatus{
							Status:    monitor.GetState().GetCurrent(),
							TimeStamp: monitor.GetState().GetStateChangeTime(),
							Group:     monitor.GetGroup(),
						}
					}
				}
			} else {
				wg.Done()
				return nil
			}

			b, err := json.Marshal(monitorsStatus)
			if err != nil {
				log.Error(err, "WS: Failed to marshal monitor status")
				return err
			}
			err = wsutil.WriteServerMessage(conn, op, b)
			if err != nil {
				log.Error(err, "WS: Failed to write server message")
				stop()
				return err
			}

			// Reading the data from the websocket connection.
			go func() {
				for {
					_, _, err := wsutil.ReadClientData(conn)
					if err != nil {
						stop()
						return
					}
				}
			}()

			// Subscribing to the monitor state changes and sending the state changes to the client.
			go func() {
				for monitorName, monitor := range configuredMonitors {
					func(monitorName string, monitor monitors.Monitor) {
						monitor.GetState().Subscribe(rid, func(state *monitors.State) {
							if monitor.IsEnabled() {
								singleMonitorStatus := make(map[string]interface{})
								singleMonitorStatus[monitor.GetName().String()] = &monitorWSStatus{
									Status:    state.GetCurrent(),
									TimeStamp: state.GetStateChangeTime(),
									Group:     monitor.GetGroup(),
								}
								b, err := json.Marshal(singleMonitorStatus)
								if err != nil {
									log.Error(err, "WS: Failed to marshal monitor status")
									return
								}
								err = wsutil.WriteServerMessage(conn, op, b)
								if err != nil {
									log.Error(err, "WS: Failed to write server message")
									return
								}
							}
						})
					}(monitorName, monitor)
				}
			}()
			// goroutine that will wait for the duration of the timeout and then call the stop
			// function.
			if viper.GetDuration("metrics.monitostatus.wsTTL") > 0 {
				go func() {
					wsTTL := viper.GetDuration("metrics.monitostatus.wsTTL")
					if wsTTL < 30*time.Second {
						wsTTL = 30 * time.Second
						log.Info("WS: Setting wsTTL to the minimum value of 30 seconds")
					}
					<-time.NewTimer(wsTTL).C
					stop()
				}()
			}

			wg.Wait()
			log.Debug("WS: Connection closed: ", rid)
			return nil
		})
		if viper.GetBool("metrics.monitostatus.ui") {
			assetHandler := http.FileServer(utils.GetFileSystem(isLive, public))
			root.GET("/static/*", echo.WrapHandler(http.StripPrefix("/static/", assetHandler)))
			root.GET("/", func(c echo.Context) error {
				return c.Render(http.StatusOK, "index.html", map[string]interface{}{
					"orgName":    viper.GetString("orgName"),
					"orgLogoURI": viper.GetString("orgLogoURI"),
					"orgURI":     viper.GetString("orgURI"),
				})
			})
		}
		log.Info("Monitostatus Websocket enabled on /api/monitors/ws")
	}
	if isMetricsEnabled {
		go func() {
			log.Info("Metrics enabled on port: ", metricsPort)
			err := webApp.Start(metricsServerString)
			if err != nil {
				log.Debug("Metrics server failed to start: ", err.Error())
			}
		}()
	}

	// End Metrics Setup

	go func() {
		<-stopper
		log.Info("Stopping monito...")

		go func() {
			for _, monitor := range configuredMonitors {
				if monitor.IsEnabled() {
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
	err = webApp.Shutdown(context.Background())
	if err != nil {
		log.Error(err, "Failed to gracefully shutdown web server")
	}
	log.Info("Exiting.")
}

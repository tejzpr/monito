package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tejzpr/monito/log"
	"github.com/tejzpr/monito/monitors"
	"github.com/tejzpr/monito/utils"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
)

// HTTP is the config for the HTTP monitor
type HTTP struct {
	// URL is the URL to monitor
	URL string `json:"url"`
	// Method is the HTTP method to use
	Method string `json:"method"`
	// Headers is the headers to send with the request
	Headers map[string]string `json:"headers"`
	// Body is the body to send with the request
	Body string `json:"body"`
	// ExpectedStatusCode is the expected status code
	ExpectedStatusCode int `json:"expectedStatusCode"`
	// ExpectedBody is the expected body
	ExpectedBody string `json:"expectedBody"`
}

// HTTPMonitor is a monitor that monitors http endpoints
// it implements the Monitor interface
type HTTPMonitor struct {
	name                  monitors.MonitorName
	config                *HTTP
	logger                monitors.Logger
	interval              time.Duration
	timeOut               time.Duration
	enabled               bool
	stopChannel           chan bool
	wg                    sync.WaitGroup
	g                     errgroup.Group
	sem                   *semaphore.Weighted
	httpClient            *http.Client
	setupOnce             sync.Once
	notifyRate            rate.Limit
	notifyRateLimit       time.Duration
	metricsEnabled        bool
	metrics               *HTTPMetrics
	notifyLimiter         *rate.Limiter
	notifyConfig          utils.NotifyConfig
	state                 *monitors.State
	notifyHandler         monitors.NotificationHandler
	maxConcurrentRequests int
	maxRetries            int
	retryCounter          int
}

// HTTPMetrics the metrics for the monitor
type HTTPMetrics struct {
	ServiceStatusGauge prometheus.Gauge
}

// StartSericeStatusGauge initializes the service status gauge
func (hm *HTTPMetrics) StartSericeStatusGauge(name string) {
	hm.ServiceStatusGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "monito",
		Subsystem: "http_metrics",
		Name:      "is_service_up_" + name,
		Help:      "Provides status of the service, 0 = down, 1 = up",
	})
	hm.ServiceStatusGauge.Set(1)
}

// SetNotifyConfig sets the notify config for the monitor
func (m *HTTPMonitor) SetNotifyConfig(notifyConfig utils.NotifyConfig) {
	m.notifyConfig = notifyConfig
}

// GetNotifyConfig gets the notify config for the monitor
func (m *HTTPMonitor) GetNotifyConfig() utils.NotifyConfig {
	return m.notifyConfig
}

// ServiceDown handles the service down
func (hm *HTTPMetrics) ServiceDown() {
	hm.ServiceStatusGauge.Dec()
}

// ServiceUp handles the service down
func (hm *HTTPMetrics) ServiceUp() {
	hm.ServiceStatusGauge.Inc()
}

// Run starts the monitor
func (m *HTTPMonitor) Run(ctx context.Context) error {
	if !m.enabled {
		return nil
	}

	if m.name.String() == "" {
		return errors.New("Name is required")
	}
	var returnerr error
	m.setupOnce.Do(func() {
		if m.logger == nil {
			returnerr = errors.New("Logger is not set")
			return
		}

		if m.metricsEnabled {
			m.metrics = &HTTPMetrics{}
			m.metrics.StartSericeStatusGauge(m.name.String())
		}

		if m.state == nil {
			m.state = &monitors.State{
				Current:         monitors.StateStatusOK,
				Previous:        monitors.StateStatusInit,
				StateChangeTime: time.Now(),
			}
		}
		m.stopChannel = make(chan bool)
		if m.sem == nil {
			m.maxConcurrentRequests = 1
			m.sem = semaphore.NewWeighted(int64(1))
		}

		if m.httpClient == nil {
			t := http.DefaultTransport.(*http.Transport).Clone()
			t.MaxIdleConns = m.maxConcurrentRequests + 1
			t.MaxConnsPerHost = m.maxConcurrentRequests + 1
			t.MaxIdleConnsPerHost = m.maxConcurrentRequests + 1
			m.httpClient = &http.Client{
				Timeout:   m.timeOut,
				Transport: t,
			}
		}

		m.logger.Infof("Started monitor %s", m.name)
		for {
			select {
			case <-ctx.Done():
				m.logger.Debugf("Stopping monitor context cancelled%s", m.name)
				return
			case <-m.stopChannel:
				m.logger.Debugf("Stopping monitor %s", m.name)
				return
			case <-time.After(m.interval):
				m.logger.Debugf("Aquire semaphore %s", m.name)
				if err := m.sem.Acquire(ctx, 1); err != nil {
					m.logger.Error(err)
					continue
				}

				m.logger.Debugf("Running monitor: %s", m.name)
				err := m.run()
				if err != nil {
					if m.state.Get().Current == monitors.StateStatusOK {
						m.metrics.ServiceDown()
						m.state.Update(monitors.StateStatusError)
					}
					m.logger.Debugf("Error running monitor [%s]: %s", m.name, err.Error())
					if m.maxRetries > 0 && m.retryCounter < m.maxRetries {
						m.retryCounter++
						m.logger.Infof("Retrying monitor: %s", m.name)
						continue
					} else if m.maxRetries > 0 && m.retryCounter >= m.maxRetries {
						m.logger.Infof("Max retries reached for monitor: %s", m.name)
						returnerr = err
						// TODO Handle retries
						return
					} else {
						continue
					}
				} else {
					if m.state.Current == monitors.StateStatusError {
						m.metrics.ServiceUp()
						m.state.Update(monitors.StateStatusOK)
						m.notifyHandler(m, nil)
						m.resetNotifyLimiter()
					}
				}
			}
		}
	})
	return returnerr
}

// SetNotifyHandler sets the notify handler for the monitor
func (m *HTTPMonitor) SetNotifyHandler(notifyHandler monitors.NotificationHandler) {
	m.notifyHandler = notifyHandler
}

// HandleFailure handles a failure
func (m *HTTPMonitor) HandleFailure(err error) error {
	m.logger.Debugf("Monitor failed with error %s", err.Error())
	if m.state.Get().Current == monitors.StateStatusOK {
		m.metrics.ServiceDown()
		m.state.Update(monitors.StateStatusError)
	}
	if m.notifyHandler != nil {
		if m.notifyLimiter == nil {
			m.notifyHandler(m, err)
		} else if m.notifyLimiter.Allow() {
			m.notifyHandler(m, err)
		}
	}
	return err
}

// run runs the monitor
func (m *HTTPMonitor) run() error {
	defer m.sem.Release(1)

	m.logger.Debugf("Running HTTP Request for monitor: %s", m.name)
	req, err := http.NewRequest(m.config.Method, m.config.URL, bytes.NewBuffer([]byte(m.config.Body)))
	if err != nil {
		return err
	}

	req.Header.Add("User-Agent", "Monito")
	for k, v := range m.config.Headers {
		req.Header.Add(k, v)
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		switch t := err.(type) {
		case *net.OpError:
			if t.Op == "dial" {
				return m.HandleFailure(errors.New("Unknown host"))
			} else if t.Op == "read" {
				return m.HandleFailure(errors.New("Connection refused [read]"))
			}
			return err
		case *url.Error:
			if t.Op == "Get" || t.Op == "Post" || t.Op == "Option" {
				return m.HandleFailure(errors.New("Connection refused [url error]"))
			}
			return err
		case syscall.Errno:
			if t == syscall.ECONNREFUSED {
				return m.HandleFailure(errors.New("Connection refused [ECONNREFUSED]"))
			}
			return err
		default:
			return err
		}
	} else if resp.StatusCode != m.config.ExpectedStatusCode {
		return m.HandleFailure(errors.New("Status code does not match"))
	}

	if m.config.ExpectedBody != "" {
		if resp.Body == nil {
			return m.HandleFailure(errors.New("Expected body but got nil"))
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return m.HandleFailure(err)
		}
		if string(body) != m.config.ExpectedBody {
			return m.HandleFailure(errors.New("Expected body does not match"))
		}
	}

	return nil
}

// Name returns the name of the monitor
func (m *HTTPMonitor) Name() monitors.MonitorName {
	return m.name
}

// Type returns the type of the monitor
func (m *HTTPMonitor) Type() monitors.MonitorType {
	return monitors.MonitorType("http")
}

// SetName sets the name of the monitor
func (m *HTTPMonitor) SetName(name monitors.MonitorName) {
	m.name = name
}

// Config returns the config for the monitor
func (m *HTTPMonitor) Config() interface{} {
	return m.config
}

// SetConfig sets the config for the monitor
func (m *HTTPMonitor) SetConfig(config interface{}) error {
	conf := config.(*HTTP)
	if conf.URL == "" {
		return errors.New("URL is required")
	} else if conf.Method == "" {
		return errors.New("Method is required")
	} else if conf.ExpectedStatusCode == 0 {
		return errors.New("ExpectedStatusCode is required")
	}
	m.config = conf
	return nil
}

// Logger returns the logger for the monitor
func (m *HTTPMonitor) Logger() monitors.Logger {
	return m.logger
}

// SetLogger sets the logger for the monitor
func (m *HTTPMonitor) SetLogger(logger monitors.Logger) {
	m.logger = logger
}

// Interval returns the interval for the monitor
// HTTP Requests are sent only after previous request has completed
func (m *HTTPMonitor) Interval() time.Duration {
	return m.interval
}

// SetInterval sets the interval for the monitor
func (m *HTTPMonitor) SetInterval(interval time.Duration) {
	m.interval = interval
}

// Enabled returns the enabled flag for the monitor
func (m *HTTPMonitor) Enabled() bool {
	return m.enabled
}

// SetEnabled sets the enabled flag for the monitor
func (m *HTTPMonitor) SetEnabled(enabled bool) {
	m.enabled = enabled
}

// TimeOut returns the timeout for the monitor
func (m *HTTPMonitor) TimeOut() time.Duration {
	return m.timeOut
}

// SetTimeOut sets the timeout for the monitor
func (m *HTTPMonitor) SetTimeOut(timeOut time.Duration) {
	m.timeOut = timeOut
}

// SetMaxConcurrentRequests sets the max concurrent requests for the monitor
func (m *HTTPMonitor) SetMaxConcurrentRequests(maxConcurrentRequests int) {
	m.maxConcurrentRequests = maxConcurrentRequests
	m.sem = semaphore.NewWeighted(int64(m.maxConcurrentRequests))
}

// SetMaxRetries sets the max retries for the monitor
func (m *HTTPMonitor) SetMaxRetries(maxRetries int) {
	m.maxRetries = maxRetries
}

// SetNotifyRateLimit sets the notify rate limit for the monitor
func (m *HTTPMonitor) SetNotifyRateLimit(notifyRateLimit time.Duration) {
	m.notifyRateLimit = notifyRateLimit
	m.notifyRate = rate.Every(notifyRateLimit)
	m.resetNotifyLimiter()
}

func (m *HTTPMonitor) resetNotifyLimiter() {
	m.notifyLimiter = rate.NewLimiter(m.notifyRate, 1)
}

// Stop stops the monitor
func (m *HTTPMonitor) Stop() {
	m.logger.Info("Stopping: ", m.name.String())
	m.stopChannel <- true
}

// GetState returns the state of the monitor
func (m *HTTPMonitor) GetState() *monitors.State {
	return m.state.Get()
}

// SetEnableMetrics sets the enable metrics flag for the monitor
func (m *HTTPMonitor) SetEnableMetrics(enableMetrics bool) {
	m.metricsEnabled = enableMetrics
}

// GetRecoveryNotificationBody returns the recovery notification body
func (m *HTTPMonitor) GetRecoveryNotificationBody() string {
	loc, _ := time.LoadLocation("UTC")
	return fmt.Sprintf("Recovered in monitor [%s]: %s \nType: %s\nRecovered URL: %s\nRecovered On: %s",
		m.Name(),
		m.state.Current,
		m.Type().String(),
		m.config.URL,
		m.state.StateChangeTime.In(loc).Format(time.RFC1123))
}

// GetErrorNotificationBody returns the error notification body
func (m *HTTPMonitor) GetErrorNotificationBody(monitorerr error) string {
	loc, _ := time.LoadLocation("UTC")
	now := time.Now()
	return fmt.Sprintf("Failure in monitor [%s]: %s \nType: %s\nFailed URL: %s\nAlerted On: %s\nNext Possible Alert In: %s",
		m.Name(),
		monitorerr.Error(),
		m.Type().String(),
		m.config.URL,
		now.In(loc).Format(time.RFC1123),
		m.notifyRateLimit.String())
}

// HTTPConfigHeader is the header for the HTTP config
type HTTPConfigHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HTTPJSONConfig is the config for the HTTP monitor
type HTTPJSONConfig struct {
	Name                  monitors.MonitorName `json:"name"`
	URL                   string               `json:"url"`
	Method                string               `json:"method"`
	Headers               HTTPConfigHeader     `json:"headers"`
	ExpectedStatusCode    int                  `json:"expectedStatusCode"`
	ExpectedResponseBody  string               `json:"expectedResponseBody"`
	Interval              utils.Duration       `json:"interval"`
	Timeout               utils.Duration       `json:"timeout"`
	MaxConcurrentRequests int                  `json:"maxConcurrentRequests"`
	MaxRetries            int                  `json:"maxRetries"`
	NotifyRateLimit       utils.Duration       `json:"notifyRateLimit"`
	NotifyDetails         utils.NotifyConfig   `json:"notifyDetails"`
}

// Validate validates the config for the HTTP monitor
func (m *HTTPJSONConfig) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("Monitor name is required")
	}
	if m.Interval.Duration <= 0 {
		m.Interval.Duration = time.Second * 2
	}

	if m.Timeout.Duration <= 0 {
		m.Timeout.Duration = time.Second * 5
	}

	if m.MaxConcurrentRequests <= 0 {
		m.MaxConcurrentRequests = 1
	}

	if m.MaxRetries <= 0 {
		m.MaxRetries = 0
	}

	if m.NotifyRateLimit.Duration <= 0 {
		m.NotifyRateLimit.Duration = 0
	}
	return nil
}

// newHTTPMonitor creates a new HTTP monitor
func newHTTPMonitor(configBody []byte, notifyHandler monitors.NotificationHandler, logger monitors.Logger, metricsEnabled bool) (monitors.Monitor, error) {
	var mConfig HTTPJSONConfig
	if err := json.Unmarshal(configBody, &mConfig); err != nil {
		log.Errorf(err, "Error unmarshalling config for monitor: http")
		return nil, err
	}
	err := mConfig.Validate()
	if err != nil {
		return nil, err
	}

	httpConfig := &HTTP{
		URL:                mConfig.URL,
		Method:             mConfig.Method,
		ExpectedBody:       mConfig.ExpectedResponseBody,
		ExpectedStatusCode: mConfig.ExpectedStatusCode,
	}

	httpMonitor := &HTTPMonitor{}
	httpMonitor.SetName(mConfig.Name)
	err = httpMonitor.SetConfig(httpConfig)
	if err != nil {
		return nil, err
	}
	httpMonitor.SetLogger(logger)
	httpMonitor.SetEnabled(true)
	httpMonitor.SetInterval(mConfig.Interval.Duration)
	httpMonitor.SetTimeOut(mConfig.Timeout.Duration)
	httpMonitor.SetMaxConcurrentRequests(mConfig.MaxConcurrentRequests)
	httpMonitor.SetMaxRetries(mConfig.MaxRetries)
	httpMonitor.SetNotifyRateLimit(mConfig.NotifyRateLimit.Duration)
	httpMonitor.SetNotifyHandler(notifyHandler)
	httpMonitor.SetEnableMetrics(metricsEnabled)
	httpMonitor.SetNotifyConfig(mConfig.NotifyDetails)
	return httpMonitor, nil
}

func init() {
	err := monitors.RegisterMonitor("http", newHTTPMonitor)
	if err != nil {
		panic(err)
	}
}

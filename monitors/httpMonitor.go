package monitors

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
	name                  MonitorName
	config                *HTTP
	logger                Logger
	interval              time.Duration
	timeOut               time.Duration
	enabled               bool
	stopChannel           chan bool
	wg                    sync.WaitGroup
	g                     errgroup.Group
	sem                   *semaphore.Weighted
	httpClient            *http.Client
	setupOnce             sync.Once
	notifyRateLimit       rate.Limit
	metricsEnabled        bool
	metrics               *HTTPMetrics
	notifyLimiter         *rate.Limiter
	state                 *State
	notifyHandler         func(state *State, err error)
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
			m.state = &State{
				Current:         StateStatusOK,
				Previous:        StateStatusInit,
				StateChangeTime: time.Now(),
			}
		}
		m.stopChannel = make(chan bool)
		if m.sem == nil {
			m.maxConcurrentRequests = 1
			m.sem = semaphore.NewWeighted(int64(1))
		}
		if m.timeOut == 0 {
			m.timeOut = time.Second * 5
		}
		if m.interval == 0 {
			m.interval = time.Second * 2
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
				if err := m.sem.Acquire(ctx, 1); err != nil {
					m.logger.Error(err)
					continue
				}
				m.logger.Debugf("Running monitor: %s", m.name)
				err := m.run()
				if err != nil {
					if m.state.Get().Current == StateStatusOK {
						m.metrics.ServiceDown()
						m.state.Update(StateStatusError)
					}
					m.logger.Debugf("Error running monitor [%s]: %s", m.name, err.Error())
					if m.maxRetries > 0 && m.retryCounter < m.maxRetries {
						m.retryCounter++
						m.logger.Infof("Retrying monitor: %s", m.name)
						continue
					} else if m.maxRetries > 0 && m.retryCounter >= m.maxRetries {
						m.logger.Infof("Max retries reached for monitor: %s", m.name)
						returnerr = err
						return
					} else {
						continue
					}
				} else {
					if m.state.Current == StateStatusError {
						m.metrics.ServiceUp()
						m.state.Update(StateStatusOK)
						m.notifyHandler(m.state, nil)
						m.resetNotifyLimiter()
					}
				}
			}
		}
	})
	return returnerr
}

// SetNotifyHandler sets the notify handler for the monitor
func (m *HTTPMonitor) SetNotifyHandler(notifyHandler func(state *State, err error)) {
	m.notifyHandler = notifyHandler
}

// HandleFailure handles a failure
func (m *HTTPMonitor) HandleFailure(err error) error {
	m.logger.Debugf("Monitor failed with error %s", err.Error())
	if m.state.Get().Current == StateStatusOK {
		m.metrics.ServiceDown()
		m.state.Update(StateStatusError)
	}
	if m.notifyHandler != nil {
		if m.notifyLimiter == nil {
			m.notifyHandler(m.state, err)
		} else if m.notifyLimiter.Allow() {
			m.notifyHandler(m.state, err)
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
func (m *HTTPMonitor) Name() MonitorName {
	return m.name
}

// SetName sets the name of the monitor
func (m *HTTPMonitor) SetName(name MonitorName) {
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
func (m *HTTPMonitor) Logger() Logger {
	return m.logger
}

// SetLogger sets the logger for the monitor
func (m *HTTPMonitor) SetLogger(logger Logger) {
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
	if maxConcurrentRequests <= 0 {
		m.maxConcurrentRequests = 1
	} else {
		m.maxConcurrentRequests = maxConcurrentRequests
	}
	m.sem = semaphore.NewWeighted(int64(m.maxConcurrentRequests))
}

// SetMaxRetries sets the max retries for the monitor
func (m *HTTPMonitor) SetMaxRetries(maxRetries int) {
	if maxRetries < 0 {
		maxRetries = 0
	}
	m.maxRetries = maxRetries
}

// SetNotifyRateLimit sets the notify rate limit for the monitor
func (m *HTTPMonitor) SetNotifyRateLimit(notifyRateLimit time.Duration) {
	if notifyRateLimit < 0 {
		notifyRateLimit = 0
	}
	m.notifyRateLimit = rate.Every(notifyRateLimit)
	m.resetNotifyLimiter()
}

func (m *HTTPMonitor) resetNotifyLimiter() {
	m.notifyLimiter = rate.NewLimiter(m.notifyRateLimit, 1)
}

// Stop stops the monitor
func (m *HTTPMonitor) Stop() {
	m.logger.Info("Stopping: ", m.name.String())
	m.stopChannel <- true
}

// GetState returns the state of the monitor
func (m *HTTPMonitor) GetState() *State {
	return m.state.Get()
}

// SetEnableMetrics sets the enable metrics flag for the monitor
func (m *HTTPMonitor) SetEnableMetrics(enableMetrics bool) {
	m.metricsEnabled = enableMetrics
}

// NewHTTPMonitor creates a new HTTP monitor
func NewHTTPMonitor(name MonitorName, runInterval time.Duration, timeOut time.Duration, maxConcurrentRequests int, maxRetries int, notifyRateLimit time.Duration, notifyHandler func(state *State, err error), config *HTTP, metricsEnabled bool, logger Logger) (Monitor, error) {
	httpMonitor := &HTTPMonitor{}
	httpMonitor.SetName(name)
	if config.Method == "" {
		return nil, errors.New("Method is required")
	}
	if config.URL == "" {
		return nil, errors.New("URL is required")
	}
	if config.ExpectedStatusCode == 0 {
		return nil, errors.New("ExpectedStatusCode is required")
	}
	httpMonitor.SetConfig(config)
	httpMonitor.SetLogger(logger)
	httpMonitor.SetEnabled(true)
	httpMonitor.SetInterval(runInterval)
	httpMonitor.SetTimeOut(timeOut)
	httpMonitor.SetMaxConcurrentRequests(maxConcurrentRequests)
	httpMonitor.SetMaxRetries(maxRetries)
	httpMonitor.SetNotifyRateLimit(notifyRateLimit)
	httpMonitor.SetNotifyHandler(notifyHandler)
	httpMonitor.SetEnableMetrics(metricsEnabled)
	return httpMonitor, nil
}

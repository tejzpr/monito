package monitors

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// Language: go
// Path: monitors/httpMonitor.go

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
	name                  string
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
	maxConcurrentRequests int
	maxRetries            int
	retryCounter          int
}

// Run starts the monitor
func (m *HTTPMonitor) Run(ctx context.Context) error {
	if !m.enabled {
		return nil
	}
	if m.name == "" {
		return errors.New("Name is required")
	}
	var returnerr error
	m.setupOnce.Do(func() {
		if m.logger == nil {
			returnerr = errors.New("Logger is not set")
			return
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
				m.logger.Infof("Stopping monitor context cancelled%s", m.name)
				return
			case <-m.stopChannel:
				m.logger.Infof("Stopping monitor %s", m.name)
				return
			case <-time.After(m.interval):
				if err := m.sem.Acquire(ctx, 1); err != nil {
					m.logger.Error(err)
					continue
				}
				m.logger.Debugf("Running monitor: %s", m.name)
				m.g.Go(m.run)
				if err := m.g.Wait(); err != nil {
					m.logger.Errorf(err, "Error running monitor: %s", m.name)
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
				}
			}
		}
	})
	return returnerr
}

// HandleFailure handles a failure
func (m *HTTPMonitor) HandleFailure(err error) {
	// Handle debounce
	m.logger.Error(err)
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
		return err
	} else if resp.StatusCode != m.config.ExpectedStatusCode {
		m.HandleFailure(err)
	}

	if m.config.ExpectedBody != "" {
		if resp.Body == nil {
			m.HandleFailure(errors.New("Expected body but got nil"))
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			m.HandleFailure(err)
		}
		if string(body) != m.config.ExpectedBody {
			m.HandleFailure(errors.New("Expected body does not match"))
		}
	}

	return nil
}

// Name returns the name of the monitor
func (m *HTTPMonitor) Name() string {
	return m.name
}

// SetName sets the name of the monitor
func (m *HTTPMonitor) SetName(name string) {
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

// Stop stops the monitor
func (m *HTTPMonitor) Stop() {
	m.stopChannel <- true
}

// NewHTTPMonitor creates a new HTTP monitor
func NewHTTPMonitor(name string, runInterval time.Duration, timeOut time.Duration, maxConcurrentRequests int, maxRetries int, config *HTTP, logger Logger) (Monitor, error) {
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
	return httpMonitor, nil
}

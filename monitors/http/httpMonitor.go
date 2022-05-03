package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"

	"github.com/tejzpr/monito/log"
	"github.com/tejzpr/monito/monitors"
	"github.com/tejzpr/monito/types"
)

// Config is the config for the HTTP monitor
type Config struct {
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

// Monitor is a monitor that monitors http endpoints
// it implements the Monitor interface
type Monitor struct {
	monitors.BaseMonitor
	httpClient *http.Client
	Config     *Config
}

// Init initializes the monitor
func (m *Monitor) Init() error {
	err := m.Initialize()
	if err != nil {
		return err
	}
	if m.httpClient == nil {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.MaxIdleConns = m.MaxConcurrentRequests + 1
		t.MaxConnsPerHost = m.MaxConcurrentRequests + 1
		t.MaxIdleConnsPerHost = m.MaxConcurrentRequests + 1
		m.httpClient = &http.Client{
			Timeout:   m.TimeOut,
			Transport: t,
		}
	}
	return nil
}

// Process runs the monitor
func (m *Monitor) Process() error {
	defer m.Semaphore.Release(1)

	m.Logger.Debugf("Running HTTP Request for monitor: %s", m.Name)
	req, err := http.NewRequest(m.Config.Method, m.Config.URL, bytes.NewBuffer([]byte(m.Config.Body)))
	if err != nil {
		return err
	}

	req.Header.Add("User-Agent", "Monito")
	for k, v := range m.Config.Headers {
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
	} else if resp.StatusCode != m.Config.ExpectedStatusCode {
		return m.HandleFailure(errors.New("Status code does not match"))
	}

	if m.Config.ExpectedBody != "" {
		if resp.Body == nil {
			return m.HandleFailure(errors.New("Expected body but got nil"))
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return m.HandleFailure(err)
		}
		if string(body) != m.Config.ExpectedBody {
			return m.HandleFailure(errors.New("Expected body does not match"))
		}
	}

	return nil
}

// GetConfig returns the config for the monitor
func (m *Monitor) GetConfig() interface{} {
	return m.Config
}

// SetConfig sets the config for the monitor
func (m *Monitor) SetConfig(config interface{}) error {
	conf := config.(*Config)
	if conf.URL == "" {
		return errors.New("URL is required")
	} else if conf.Method == "" {
		return errors.New("Method is required")
	} else if conf.ExpectedStatusCode == 0 {
		return errors.New("ExpectedStatusCode is required")
	}
	m.Config = conf
	return nil
}

// GetNotificationBody returns the notification body
func (m *Monitor) GetNotificationBody(state *monitors.State) *monitors.NotificationBody {
	loc, _ := time.LoadLocation("UTC")
	now := time.Now()
	return &monitors.NotificationBody{
		Name:     m.GetName(),
		Type:     m.GetType(),
		EndPoint: m.Config.URL,
		Time:     now.In(loc),
		Status:   state.GetCurrent(),
		Error:    state.GetError(),
	}
}

// ConfigHeader is the header for the HTTP config
type ConfigHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// JSONConfig is the config for the HTTP monitor
type JSONConfig struct {
	monitors.JSONBaseConfig
	URL                  string       `json:"url"`
	Method               string       `json:"method"`
	Headers              ConfigHeader `json:"headers"`
	ExpectedStatusCode   int          `json:"expectedStatusCode"`
	ExpectedResponseBody string       `json:"expectedResponseBody"`
}

// Validate validates the config for the Port monitor
func (m *JSONConfig) monitorFieldsValidate() error {
	if m.URL == "" {
		return errors.New("URL is required")
	} else if m.Method == "" {
		return errors.New("Method is required")
	} else if m.ExpectedStatusCode == 0 {
		return errors.New("ExpectedStatusCode is required")
	}
	return nil
}

// newMonitor creates a new HTTP monitor
func newMonitor(configBody []byte, logger monitors.Logger, jitterFactor int, prometheusEnabled bool) (monitors.Monitor, error) {
	var mConfig JSONConfig
	if err := json.Unmarshal(configBody, &mConfig); err != nil {
		log.Errorf(err, "Error unmarshalling config for monitor: http")
		return nil, err
	}
	err := mConfig.Validate()
	if err != nil {
		return nil, err
	}
	err = mConfig.monitorFieldsValidate()
	if err != nil {
		return nil, err
	}
	httpConfig := &Config{
		URL:                mConfig.URL,
		Method:             mConfig.Method,
		ExpectedBody:       mConfig.ExpectedResponseBody,
		ExpectedStatusCode: mConfig.ExpectedStatusCode,
	}

	httpMonitor := &Monitor{}

	httpMonitor.SetName(mConfig.Name)
	err = httpMonitor.SetConfig(httpConfig)
	if err != nil {
		return nil, err
	}
	httpMonitor.SetProcess(httpMonitor.Process)
	httpMonitor.SetDescription(mConfig.Description)
	httpMonitor.SetGroup(mConfig.Group)
	httpMonitor.SetLogger(logger)
	httpMonitor.SetEnabled(mConfig.Enabled)
	httpMonitor.SetInterval(mConfig.Interval.Duration)
	httpMonitor.SetTimeOut(mConfig.Timeout.Duration)
	httpMonitor.SetMaxConcurrentRequests(mConfig.MaxConcurrentRequests)
	httpMonitor.SetMaxRetries(mConfig.MaxRetries)
	httpMonitor.SetNotifyRateLimit(mConfig.NotifyRateLimit.Duration)
	httpMonitor.SetEnablePrometheusMetrics(prometheusEnabled)
	httpMonitor.SetJitterFactor(jitterFactor)
	httpMonitor.SetType(types.MonitorType("http"))
	return httpMonitor, nil
}

func init() {
	err := monitors.RegisterMonitor(types.MonitorType("http"), newMonitor)
	if err != nil {
		panic(err)
	}
}

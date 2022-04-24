package port

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
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

// NetworkProtocol returns the network protocol
type NetworkProtocol string

const (
	// TCP is the tcp protocol
	TCP NetworkProtocol = "tcp"
	// UDP is the udp protocol
	UDP NetworkProtocol = "udp"
)

// GetProtocol returns the protocol
// Defaults to tcp
func (n NetworkProtocol) GetProtocol(p string) NetworkProtocol {
	p = strings.ToLower(p)
	if p == "tcp" {
		return TCP
	} else if p == "udp" {
		return UDP
	}
	return TCP
}

// String returns the string representation of the protocol
func (n NetworkProtocol) String() string {
	return string(n)
}

// Config is the config for the Port monitor
type Config struct {
	// Host is the host to monitor
	Host string `json:"host"`
	// Port is the Port to monitor
	Port uint64 `json:"port"`
	// Protocol is the protocol to use
	Protocol NetworkProtocol `json:"protocol"`
}

// Metrics the metrics for the monitor
type Metrics struct {
	ServiceStatusGauge prometheus.Gauge
}

// StartSericeStatusGauge initializes the service status gauge
func (pm *Metrics) StartSericeStatusGauge(name string) {
	pm.ServiceStatusGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "monito",
		Subsystem: "port_metrics",
		Name:      "is_service_up_" + name,
		Help:      "Provides status of the service, 0 = down, 1 = up",
	})
	pm.ServiceStatusGauge.Set(1)
}

// ServiceDown handles the service down
func (pm *Metrics) ServiceDown() {
	pm.ServiceStatusGauge.Dec()
}

// ServiceUp handles the service down
func (pm *Metrics) ServiceUp() {
	pm.ServiceStatusGauge.Inc()
}

// Monitor is a monitor that monitors ports
// it implements the Monitor interface
type Monitor struct {
	name                  monitors.MonitorName
	config                *Config
	logger                monitors.Logger
	interval              time.Duration
	timeOut               time.Duration
	enabled               bool
	stopChannel           chan bool
	wg                    sync.WaitGroup
	g                     errgroup.Group
	sem                   *semaphore.Weighted
	setupOnce             sync.Once
	notifyRate            rate.Limit
	notifyRateLimit       time.Duration
	metricsEnabled        bool
	metrics               *Metrics
	notifyLimiter         *rate.Limiter
	notifyConfig          utils.NotifyConfig
	state                 *monitors.State
	notifyHandler         monitors.NotificationHandler
	maxConcurrentRequests int
	maxRetries            int
	retryCounter          int
}

// CheckPort checks if a port is open
func (m *Monitor) CheckPort() error {
	conn, err := net.DialTimeout(m.config.Protocol.String(), net.JoinHostPort(m.config.Host, strconv.FormatUint(m.config.Port, 10)), m.timeOut)
	if err != nil {
		return err
	}
	if conn != nil {
		defer conn.Close()
		return nil
	}
	return nil
}

// Run starts the monitor
func (m *Monitor) Run(ctx context.Context) error {
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
			m.metrics = &Metrics{}
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
func (m *Monitor) SetNotifyHandler(notifyHandler monitors.NotificationHandler) {
	m.notifyHandler = notifyHandler
}

// HandleFailure handles a failure
func (m *Monitor) HandleFailure(err error) error {
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
func (m *Monitor) run() error {
	defer m.sem.Release(1)

	m.logger.Debugf("Running Port Request for monitor: %s", m.name)

	err := m.CheckPort()
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
	}

	return nil
}

// Name returns the name of the monitor
func (m *Monitor) Name() monitors.MonitorName {
	return m.name
}

// Type returns the type of the monitor
func (m *Monitor) Type() monitors.MonitorType {
	return monitors.MonitorType("port")
}

// SetName sets the name of the monitor
func (m *Monitor) SetName(name monitors.MonitorName) {
	m.name = name
}

// Config returns the config for the monitor
func (m *Monitor) Config() interface{} {
	return m.config
}

// SetConfig sets the config for the monitor
func (m *Monitor) SetConfig(config interface{}) error {
	conf := config.(*Config)
	if conf.Host == "" {
		return errors.New("Host is required")
	} else if conf.Protocol.String() == "" {
		return errors.New("Protocol is required")
	} else if conf.Port == 0 {
		return errors.New("Port is required")
	}
	m.config = conf
	return nil
}

// Logger returns the logger for the monitor
func (m *Monitor) Logger() monitors.Logger {
	return m.logger
}

// SetLogger sets the logger for the monitor
func (m *Monitor) SetLogger(logger monitors.Logger) {
	m.logger = logger
}

// Interval returns the interval for the monitor
// HTTP Requests are sent only after previous request has completed
func (m *Monitor) Interval() time.Duration {
	return m.interval
}

// SetInterval sets the interval for the monitor
func (m *Monitor) SetInterval(interval time.Duration) {
	m.interval = interval
}

// Enabled returns the enabled flag for the monitor
func (m *Monitor) Enabled() bool {
	return m.enabled
}

// SetEnabled sets the enabled flag for the monitor
func (m *Monitor) SetEnabled(enabled bool) {
	m.enabled = enabled
}

// TimeOut returns the timeout for the monitor
func (m *Monitor) TimeOut() time.Duration {
	return m.timeOut
}

// SetTimeOut sets the timeout for the monitor
func (m *Monitor) SetTimeOut(timeOut time.Duration) {
	m.timeOut = timeOut
}

// SetMaxConcurrentRequests sets the max concurrent requests for the monitor
func (m *Monitor) SetMaxConcurrentRequests(maxConcurrentRequests int) {
	m.maxConcurrentRequests = maxConcurrentRequests
	m.sem = semaphore.NewWeighted(int64(m.maxConcurrentRequests))
}

// SetMaxRetries sets the max retries for the monitor
func (m *Monitor) SetMaxRetries(maxRetries int) {
	m.maxRetries = maxRetries
}

// SetNotifyRateLimit sets the notify rate limit for the monitor
func (m *Monitor) SetNotifyRateLimit(notifyRateLimit time.Duration) {
	m.notifyRateLimit = notifyRateLimit
	m.notifyRate = rate.Every(notifyRateLimit)
	m.resetNotifyLimiter()
}

func (m *Monitor) resetNotifyLimiter() {
	m.notifyLimiter = rate.NewLimiter(m.notifyRate, 1)
}

// SetNotifyConfig sets the notify config for the monitor
func (m *Monitor) SetNotifyConfig(notifyConfig utils.NotifyConfig) {
	m.notifyConfig = notifyConfig
}

// GetNotifyConfig gets the notify config for the monitor
func (m *Monitor) GetNotifyConfig() utils.NotifyConfig {
	return m.notifyConfig
}

// Stop stops the monitor
func (m *Monitor) Stop() {
	m.logger.Info("Stopping: ", m.name.String())
	m.stopChannel <- true
}

// GetState returns the state of the monitor
func (m *Monitor) GetState() *monitors.State {
	return m.state.Get()
}

// SetEnableMetrics sets the enable metrics flag for the monitor
func (m *Monitor) SetEnableMetrics(enableMetrics bool) {
	m.metricsEnabled = enableMetrics
}

// GetRecoveryNotificationBody returns the recovery notification body
func (m *Monitor) GetRecoveryNotificationBody() string {
	loc, _ := time.LoadLocation("UTC")
	return fmt.Sprintf("Recovered in monitor [%s]: %s \nType: %s\nRecovered [protocol://host:port]: %s://%s:%d\nRecovered On: %s",
		m.Name(),
		m.state.Current,
		m.Type().String(),
		m.config.Protocol.String(),
		m.config.Host,
		m.config.Port,
		m.state.StateChangeTime.In(loc).Format(time.RFC1123))
}

// GetErrorNotificationBody returns the error notification body
func (m *Monitor) GetErrorNotificationBody(monitorerr error) string {
	loc, _ := time.LoadLocation("UTC")
	now := time.Now()
	return fmt.Sprintf("Failure in monitor [%s]: %s \nType: %s\nFailed [protocol://host:port]: %s://%s:%d\nAlerted On: %s\nNext Possible Alert In: %s",
		m.Name(),
		monitorerr.Error(),
		m.Type().String(),
		m.config.Protocol.String(),
		m.config.Host,
		m.config.Port,
		now.In(loc).Format(time.RFC1123),
		m.notifyRateLimit.String())
}

// JSONConfig is the config for the Ping monitor
type JSONConfig struct {
	monitors.JSONBaseConfig
	Host     string          `json:"host"`
	Port     uint64          `json:"port"`
	Protocol NetworkProtocol `json:"protocol"`
}

// Validate validates the config for the Port monitor
func (m *JSONConfig) Validate() error {
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

// newPortMonitor creates a new Port monitor
func newPortMonitor(configBody []byte, notifyHandler monitors.NotificationHandler, logger monitors.Logger, metricsEnabled bool) (monitors.Monitor, error) {
	var mConfig JSONConfig
	if err := json.Unmarshal(configBody, &mConfig); err != nil {
		log.Errorf(err, "Error unmarshalling config for monitor: http")
		return nil, err
	}
	err := mConfig.Validate()
	if err != nil {
		return nil, err
	}

	portConfig := &Config{
		Host:     mConfig.Host,
		Port:     mConfig.Port,
		Protocol: mConfig.Protocol,
	}

	portMonitor := &Monitor{}
	portMonitor.SetName(mConfig.Name)
	err = portMonitor.SetConfig(portConfig)
	if err != nil {
		return nil, err
	}
	portMonitor.SetLogger(logger)
	portMonitor.SetEnabled(mConfig.Enabled)
	portMonitor.SetInterval(mConfig.Interval.Duration)
	portMonitor.SetTimeOut(mConfig.Timeout.Duration)
	portMonitor.SetMaxConcurrentRequests(mConfig.MaxConcurrentRequests)
	portMonitor.SetMaxRetries(mConfig.MaxRetries)
	portMonitor.SetNotifyRateLimit(mConfig.NotifyRateLimit.Duration)
	portMonitor.SetNotifyHandler(notifyHandler)
	portMonitor.SetEnableMetrics(metricsEnabled)
	portMonitor.SetNotifyConfig(mConfig.NotifyDetails)
	return portMonitor, nil
}

func init() {
	err := monitors.RegisterMonitor("port", newPortMonitor)
	if err != nil {
		panic(err)
	}
}

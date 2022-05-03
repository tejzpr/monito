package monitors

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/tejzpr/monito/metrics"
	"github.com/tejzpr/monito/types"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
)

// MonitorProcess is the process for the monitor
type MonitorProcess func() error

// Monitor is an interface that all monoitors must implement
type Monitor interface {
	// Init initializes the monitor
	Init() error
	// Run starts the monitor
	Run(ctx context.Context) error
	// SetProcess sets the process for the monitor
	// Have to use this method to pass Process function to embedded struct
	SetProcess(p MonitorProcess)
	// Stop stops the monitor
	Stop()
	// GetName returns the name of the monitor
	GetName() MonitorName
	// SetName sets the name of the monitor
	SetName(name MonitorName)
	// Group returns the group of the monitor
	GetGroup() string
	// SetGroup sets the group of the monitor
	SetGroup(group string)
	// GetDescription returns the description of the monitor
	GetDescription() string
	// SetDescription sets the description of the monitor
	SetDescription(description string)
	// SetType sets the type of the monitor
	SetType(monitorType types.MonitorType)
	// GetType returns the type of the monitor
	GetType() types.MonitorType
	// SetConfig sets the config for the monitor
	SetConfig(config interface{}) error
	// GetConfig returns the config for the monitor
	GetConfig() interface{}
	// SetLogger sets the logger for the monitor
	SetLogger(logger Logger)
	// GetLogger returns the logger for the monitor
	GetLogger() Logger
	// SetInterval sets the interval for the monitor
	SetInterval(interval time.Duration)
	// GetInterval returns the interval for the monitor
	GetInterval() time.Duration
	// SetEnabled sets the enabled flag for the monitor
	SetEnabled(enabled bool)
	// SetJitterFactor sets the jitter factor for the monitor
	SetJitterFactor(jitterFactor int)
	// IsEnabled returns the enabled flag for the monitor
	IsEnabled() bool
	// SetMaxConcurrentRequests sets the max concurrent requests for the monitor
	SetMaxConcurrentRequests(maxConcurrentRequests int)
	// SetTimeOut sets the timeout for the monitor
	SetTimeOut(timeOut time.Duration)
	// GetTimeOut returns the timeout for the monitor
	GetTimeOut() time.Duration
	// SetMaxRetries sets the max retries for the monitor
	SetMaxRetries(maxRetries int)
	// SetNotifyRateLimit sets the notify rate limit for the monitor
	SetNotifyRateLimit(notifyRateLimit time.Duration)
	// GetState returns the state of the monitor
	GetState() *State
	// SetEnablePrometheusMetrics sets the metrics enabled flag for the monitor
	SetEnablePrometheusMetrics(enablePrometheusMetrics bool)
	// GetNotificationBody returns the error notification body
	GetNotificationBody(state *State) *NotificationBody
}

// BaseMonitor is the base monitor struct that all monitors should be composed of
type BaseMonitor struct {
	Name                     MonitorName
	Type                     types.MonitorType
	Description              string
	Group                    string
	Logger                   Logger
	Interval                 time.Duration
	TimeOut                  time.Duration
	JitterFactor             int
	Enabled                  bool
	StopChannel              chan bool
	Semaphore                *semaphore.Weighted
	InitOnce                 sync.Once
	NotifyRate               rate.Limit
	PrometheusMetricsEnabled bool
	PrometheusMetrics        *metrics.PrometheusMetrics
	NotifyLimiter            *rate.Limiter
	State                    *State
	MaxConcurrentRequests    int
	MaxRetries               int
	RetryCounter             int
	Process                  func() error
}

// Initialize the base monitor
// Call this method in all Init() methods of all monitors that are composed of BaseMonitor
func (m *BaseMonitor) Initialize() error {
	var returnerr error

	if !m.Enabled {
		returnerr = errors.New("Monitor is not enabled")
		return returnerr
	}

	if m.Type.String() == "" {
		returnerr = errors.New("Monitor type is not set")
		return returnerr
	}

	if m.Name.String() == "" {
		returnerr = errors.New("Name is required")
		return returnerr
	}

	if m.Description == "" {
		returnerr = errors.New("Description is required")
		return returnerr
	}

	if m.Interval == 0 {
		returnerr = errors.New("Interval is required")
		return returnerr
	}

	if m.TimeOut == 0 {
		returnerr = errors.New("TimeOut is required")
		return returnerr
	}

	if m.MaxConcurrentRequests == 0 {
		returnerr = errors.New("MaxConcurrentRequests is required")
		return returnerr
	}

	if m.NotifyRate == 0 {
		returnerr = errors.New("NotifyRate is required")
		return returnerr
	}

	if m.Logger == nil {
		returnerr = errors.New("Logger is not set")
		return returnerr
	}

	if m.PrometheusMetricsEnabled {
		m.PrometheusMetrics = &metrics.PrometheusMetrics{}
		m.PrometheusMetrics.StartServiceStatusGauge(m.Name.String(), m.Group, m.Type)
		m.PrometheusMetrics.StartLatencyGauge(m.Name.String(), m.Group, m.Type)
	}

	if m.State == nil {
		state := &State{}
		state.Init(StateStatusINIT, StateStatusSTARTING, time.Now())
		m.State = state
	}
	m.StopChannel = make(chan bool)
	if m.Semaphore == nil {
		m.MaxConcurrentRequests = 1
		m.Semaphore = semaphore.NewWeighted(int64(1))
	}

	return returnerr
}

// SetJitterFactor sets the jitter factor for the monitor
func (m *BaseMonitor) SetJitterFactor(jitterFactor int) {
	m.JitterFactor = jitterFactor
}

// SetJitterFactor sets the jitter factor for the monitor
func (m *BaseMonitor) getJitterFactor() int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(m.JitterFactor + 1)
}

// SetType sets the type of the monitor
func (m *BaseMonitor) SetType(monitorType types.MonitorType) {
	m.Type = monitorType
}

// GetType returns the type of the monitor
func (m *BaseMonitor) GetType() types.MonitorType {
	return m.Type
}

// HandleFailure handles a failure
func (m *BaseMonitor) HandleFailure(err error) error {
	m.Logger.Debugf("Monitor failed with error %s", err.Error())
	if m.State.IsCurrentStatusUP() {
		m.PrometheusMetrics.ServiceDown()
		m.State.Update(StateStatusDOWN)
	}
	return err
}

// SetGroup sets the group for the monitor
func (m *BaseMonitor) SetGroup(group string) {
	m.Group = group
}

// GetGroup returns the group for the monitor
func (m *BaseMonitor) GetGroup() string {
	return m.Group
}

// GetName returns the name of the monitor
func (m *BaseMonitor) GetName() MonitorName {
	return m.Name
}

// SetName sets the name of the monitor
func (m *BaseMonitor) SetName(name MonitorName) {
	m.Name = name
}

// GetDescription returns the description of the monitor
func (m *BaseMonitor) GetDescription() string {
	return m.Description
}

// SetDescription sets the description of the monitor
func (m *BaseMonitor) SetDescription(description string) {
	m.Description = description
}

// GetLogger returns the logger for the monitor
func (m *BaseMonitor) GetLogger() Logger {
	return m.Logger
}

// SetLogger sets the logger for the monitor
func (m *BaseMonitor) SetLogger(logger Logger) {
	m.Logger = logger
}

// GetInterval returns the interval for the monitor
func (m *BaseMonitor) GetInterval() time.Duration {
	return m.Interval
}

// SetInterval sets the interval for the monitor
func (m *BaseMonitor) SetInterval(interval time.Duration) {
	m.Interval = interval
}

// IsEnabled returns the enabled flag for the monitor
func (m *BaseMonitor) IsEnabled() bool {
	return m.Enabled
}

// SetEnabled sets the enabled flag for the monitor
func (m *BaseMonitor) SetEnabled(enabled bool) {
	m.Enabled = enabled
}

// GetTimeOut returns the timeout for the monitor
func (m *BaseMonitor) GetTimeOut() time.Duration {
	return m.TimeOut
}

// SetTimeOut sets the timeout for the monitor
func (m *BaseMonitor) SetTimeOut(timeOut time.Duration) {
	m.TimeOut = timeOut
}

// SetMaxConcurrentRequests sets the max concurrent requests for the monitor
func (m *BaseMonitor) SetMaxConcurrentRequests(maxConcurrentRequests int) {
	m.MaxConcurrentRequests = maxConcurrentRequests
	m.Semaphore = semaphore.NewWeighted(int64(m.MaxConcurrentRequests))
}

// SetEnablePrometheusMetrics sets the enable metrics flag for the monitor
func (m *BaseMonitor) SetEnablePrometheusMetrics(enableMetrics bool) {
	m.PrometheusMetricsEnabled = enableMetrics
}

// SetMaxRetries sets the max retries for the monitor
func (m *BaseMonitor) SetMaxRetries(maxRetries int) {
	m.MaxRetries = maxRetries
}

// Stop stops the monitor
func (m *BaseMonitor) Stop() {
	m.Logger.Info("Stopping: ", m.Name.String())
	m.StopChannel <- true
	m.Logger.Info("Send stop signal to monitor: ", m.Name.String())
}

// GetState returns the state of the monitor
func (m *BaseMonitor) GetState() *State {
	return m.State.Get()
}

// ResetNotifyLimiter resets the notify limiter for the monitor
func (m *BaseMonitor) ResetNotifyLimiter() {
	m.NotifyLimiter = rate.NewLimiter(m.NotifyRate, 1)
}

// SetNotifyRateLimit sets the notify rate limit for the monitor
func (m *BaseMonitor) SetNotifyRateLimit(notifyRateLimit time.Duration) {
	m.NotifyRate = rate.Every(notifyRateLimit)
	m.ResetNotifyLimiter()
}

// SetProcess is the main process for the monitor
func (m *BaseMonitor) SetProcess(p MonitorProcess) {
	m.Process = p
}

// Run starts the monitor
func (m *BaseMonitor) Run(ctx context.Context) error {
	m.Logger.Infof("Started monitor %s", m.Name)

	for {
		select {
		case <-ctx.Done():
			m.Logger.Debugf("Stopping monitor context cancelled%s", m.Name)
			return nil
		case <-m.StopChannel:
			m.Logger.Debugf("Stopping monitor %s", m.Name)
			return nil
		case <-time.After(m.Interval + (time.Duration(m.getJitterFactor()) * time.Second)):
			m.Logger.Debugf("Aquire semaphore %s", m.Name)
			if err := m.Semaphore.Acquire(ctx, 1); err != nil {
				m.Logger.Error(err)
				continue
			}
			m.Logger.Debugf("Running monitor: %s", m.Name)
			start := time.Now()
			err := m.Process()
			if err != nil {
				if m.State.IsCurrentStatusUP() {
					m.PrometheusMetrics.ServiceDown()
					m.State.Update(StateStatusDOWN)
				}
				m.Logger.Debugf("Error running monitor [%s]: %s", m.Name, err.Error())
				if m.MaxRetries > 0 && m.RetryCounter < m.MaxRetries {
					m.RetryCounter++
					m.Logger.Infof("Retrying monitor: %s", m.Name)
					continue
				} else if m.MaxRetries > 0 && m.RetryCounter >= m.MaxRetries {
					m.Logger.Infof("Max retries reached for monitor: %s", m.Name)
					return err
				} else {
					continue
				}
			} else {
				elapsed := time.Since(start)
				m.PrometheusMetrics.RecordLatency(elapsed)
				if m.State.IsCurrentStatusDOWN() {
					m.PrometheusMetrics.ServiceUp()
					m.State.Update(StateStatusUP)
					m.ResetNotifyLimiter()
				}
			}
		}
	}
}

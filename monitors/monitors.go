package monitors

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/tejzpr/monito/log"
	"github.com/tejzpr/monito/utils"
)

var isStringAlphabetic = regexp.MustCompile(`^[a-zA-Z0-9_]*$`).MatchString
var symbolsRegexp = regexp.MustCompile(`[^\w]`)

// MonitorName is a string that represents the name of the monitor
type MonitorName string

func (m MonitorName) String() string {
	return symbolsRegexp.ReplaceAllString(string(m), "_")
}

// MonitorType is a string that represents the type of the monitor
type MonitorType string

func (m MonitorType) String() string {
	return symbolsRegexp.ReplaceAllString(string(m), "_")
}

// NotificationHandler is the function that is called when a monitor is notified
type NotificationHandler func(monitor Monitor, err error)

// Monitor is an interface that all monoitors must implement
type Monitor interface {
	// Run starts the monitor
	Run(ctx context.Context) error
	// Stop stops the monitor
	Stop()
	// Name returns the name of the monitor
	Name() MonitorName
	// Type returns the type of the monitor
	Type() MonitorType
	// SetName sets the name of the monitor
	SetName(name MonitorName)
	// SetConfig sets the config for the monitor
	SetConfig(config interface{}) error
	// Config returns the config for the monitor
	Config() interface{}
	// SetLogger sets the logger for the monitor
	SetLogger(logger Logger)
	// Logger returns the logger for the monitor
	Logger() Logger
	// SetInterval sets the interval for the monitor
	SetInterval(interval time.Duration)
	// Interval returns the interval for the monitor
	Interval() time.Duration
	// SetEnabled sets the enabled flag for the monitor
	SetEnabled(enabled bool)
	// Enabled returns the enabled flag for the monitor
	Enabled() bool
	// SetMaxConcurrentRequests sets the max concurrent requests for the monitor
	SetMaxConcurrentRequests(maxConcurrentRequests int)
	// SetTimeOut sets the timeout for the monitor
	SetTimeOut(timeOut time.Duration)
	// TimeOut returns the timeout for the monitor
	TimeOut() time.Duration
	// SetMaxRetries sets the max retries for the monitor
	SetMaxRetries(maxRetries int)
	// SetNotifyHandler handles the notification failure of the monitor
	// Calls to this function should be non-blocking
	SetNotifyHandler(notifyHandler NotificationHandler)
	// SetNotifyRateLimit sets the notify rate limit for the monitor
	SetNotifyRateLimit(notifyRateLimit time.Duration)
	// GetState returns the state of the monitor
	GetState() *State
	// SetEnableMetrics sets the metrics enabled flag for the monitor
	SetEnableMetrics(enableMetrics bool)
	// GetErrorNotificatonBody returns the error notification body
	GetErrorNotificationBody(monitorerr error) string
	// GetRecoveryNotificationBody returns the recovery notification body
	GetRecoveryNotificationBody() string
	// SetNotifyConfig sets the notify config for the monitor
	SetNotifyConfig(notifyConfig utils.NotifyConfig)
	// GetNotifyConfig returns the notify config for the monitor
	GetNotifyConfig() utils.NotifyConfig
}

// StateStatus is the state of the monitor
type StateStatus string

const (
	// StateStatusOK indicates that the monitor is in a healthy state
	StateStatusOK StateStatus = "OK"
	// StateStatusError indicates that the monitor is in a error state
	StateStatusError StateStatus = "ERROR"
	// StateStatusInit indicates that the monitor is in a initializing state
	StateStatusInit StateStatus = "INIT"
)

// State is the state of the monitor
type State struct {
	Previous        StateStatus
	Current         StateStatus
	StateChangeTime time.Time
}

// Get returns the current state of the monitor
func (s *State) Get() *State {
	return s
}

// Update updates the state of the monitor
func (s *State) Update(newState StateStatus) error {
	// Validate newState
	if newState != StateStatusOK &&
		newState != StateStatusError &&
		newState != StateStatusInit {
		return fmt.Errorf("Invalid state: %s", newState)
	}
	s.Previous = s.Current
	s.Current = newState
	s.StateChangeTime = time.Now()
	return nil
}

// Logger is the interface of the logger for the monitor
type Logger interface {
	// Info logs an info message
	Info(msg ...interface{})
	// Infof logs an info formatted string
	Infof(format string, a ...interface{})
	// Debug logs an debug message
	Debug(msg ...interface{})
	// Debugf logs an debug formatted string
	Debugf(format string, a ...interface{})
	// Warn logs a warn message
	Warn(msg ...interface{})
	// Warnf logs a warn message
	Warnf(format string, v ...interface{})
	// Error logs an error with the message
	Error(err error, msg ...interface{})
	// Errorf logs an error with the formatted string
	Errorf(err error, format string, a ...interface{})
	// GetLevel returns the current log level
	GetLevel() log.LogLevel
	// SetLevel sets the log level
	SetLevel(level log.LogLevel)
}

var (
	monitorMu sync.RWMutex
	monitors  = make(map[string]func(configBody []byte, notifyHandler NotificationHandler, logger Logger, metricsEnabled bool) (Monitor, error))
)

// RegisterMonitor registers a monitor
func RegisterMonitor(name string, initFunc func(configBody []byte, notifyHandler NotificationHandler, logger Logger, metricsEnabled bool) (Monitor, error)) error {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	if _, dup := monitors[name]; dup {
		return fmt.Errorf("monitor is already registered: %s", name)
	}
	log.Info("Registering monitor: ", name)
	monitors[name] = initFunc
	return nil
}

// GetMonitor returns a monitor
func GetMonitor(name string, configBody []byte, notifyHandler NotificationHandler, logger Logger, metricsEnabled bool) (Monitor, error) {
	monitorMu.RLock()
	defer monitorMu.RUnlock()
	if initFunc, ok := monitors[name]; ok {
		return initFunc(configBody, notifyHandler, logger, metricsEnabled)
	}
	return nil, fmt.Errorf("monitor is not registered: %s", name)
}

// CheckIfMonitorRegistered checks the monitor
func CheckIfMonitorRegistered(name string) bool {
	monitorMu.RLock()
	defer monitorMu.RUnlock()
	_, ok := monitors[name]
	return ok
}

// GetRegisteredMonitorNames returns the registered monitor names
func GetRegisteredMonitorNames() []string {
	monitorMu.RLock()
	defer monitorMu.RUnlock()
	var names []string
	for name := range monitors {
		names = append(names, name)
	}
	return names
}

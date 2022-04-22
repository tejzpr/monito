package monitors

import (
	"context"
	"fmt"
	"time"

	"github.com/tejzpr/monito/log"
)

// Monitor is an interface that all monoitors must implement
type Monitor interface {
	// Run starts the monitor
	Run(ctx context.Context) error
	// Stop stops the monitor
	Stop()
	// Name returns the name of the monitor
	Name() string
	// SetName sets the name of the monitor
	SetName(name string)
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
	SetNotifyHandler(notifyHandler func(state *State, err error))
	// SetNotifyRateLimit sets the notify rate limit for the monitor
	SetNotifyRateLimit(notifyRateLimit time.Duration)
	// GetState returns the state of the monitor
	GetState() *State
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

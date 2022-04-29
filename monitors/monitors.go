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
// type NotificationHandler func(monitor Monitor, err error)

// JSONBaseConfig is the base JSON config for all monitors
type JSONBaseConfig struct {
	Name                  MonitorName    `json:"name"`
	Description           string         `json:"description"`
	Group                 string         `json:"group"`
	Enabled               bool           `json:"enabled"`
	Interval              utils.Duration `json:"interval"`
	Timeout               utils.Duration `json:"timeout"`
	MaxConcurrentRequests int            `json:"maxConcurrentRequests"`
	MaxRetries            int            `json:"maxRetries"`
	NotifyRateLimit       utils.Duration `json:"notifyRateLimit"`
}

// Validate validates the config for the Port monitor
func (m *JSONBaseConfig) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("Monitor name is required")
	}
	if m.Description == "" {
		return fmt.Errorf("Monitor description is required for: %s", m.Name)
	} else if len(m.Description) > 255 {
		return fmt.Errorf("Monitor description is too long for: %s", m.Name)
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

// NotificationBody is the body of the notification
type NotificationBody struct {
	Name     MonitorName `json:"name"`
	Type     MonitorType `json:"type"`
	EndPoint string      `json:"endPoint"`
	Time     time.Time   `json:"time"`
	Status   StateStatus `json:"status"`
}

// Monitor is an interface that all monoitors must implement
type Monitor interface {
	// Init initializes the monitor
	Init() error
	// Run starts the monitor
	Run(ctx context.Context) error
	// Stop stops the monitor
	Stop()
	// Name returns the name of the monitor
	Name() MonitorName
	// SetName sets the name of the monitor
	SetName(name MonitorName)
	// Group returns the group of the monitor
	Group() string
	// SetGroup sets the group of the monitor
	SetGroup(group string)
	// Description returns the description of the monitor
	Description() string
	// SetDescription sets the description of the monitor
	SetDescription(description string)
	// Type returns the type of the monitor
	Type() MonitorType
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
	// SetNotifyRateLimit sets the notify rate limit for the monitor
	SetNotifyRateLimit(notifyRateLimit time.Duration)
	// GetState returns the state of the monitor
	GetState() *State
	// SetEnableMetrics sets the metrics enabled flag for the monitor
	SetEnableMetrics(enableMetrics bool)
	// GetNotificationBody returns the error notification body
	GetNotificationBody(state *State) *NotificationBody
}

// StateStatus is the state of the monitor
type StateStatus string

const (
	// StateStatusUP indicates that the monitor is in a healthy state
	StateStatusUP StateStatus = "UP"
	// StateStatusDOWN indicates that the monitor is in a down state
	StateStatusDOWN StateStatus = "DOWN"
	// StateStatusINIT indicates that the monitor is in a initializing state
	StateStatusINIT StateStatus = "INIT"
	// StateStatusSTARTING indicates that the monitor is in a initializing state
	StateStatusSTARTING StateStatus = "STARTING"
)

// String returns the string representation of the state
func (s StateStatus) String() string {
	return string(s)
}

// State is the state of the monitor
type State struct {
	previous        StateStatus
	current         StateStatus
	stateChangeTime time.Time
	sMutex          sync.Mutex
	stateChan       chan *State
	subscribers     map[string]func(*State)
	initialized     bool
}

// Init initializes the state
func (s *State) Init(current StateStatus, previous StateStatus, stateCHangeTime time.Time) {
	s.previous = previous
	s.current = current
	s.stateChangeTime = stateCHangeTime
	s.stateChan = make(chan *State, 1)
	s.subscribers = make(map[string]func(*State))
	go func() {
		for {
			select {
			case state := <-s.stateChan:
				s.sMutex.Lock()
				for _, subscriber := range s.subscribers {
					subscriber(state)
				}
				s.sMutex.Unlock()
			}
		}
	}()
}

// Get returns the current state of the monitor
func (s *State) Get() *State {
	return s
}

// SetPrevious sets the previous state
func (s *State) SetPrevious(previous StateStatus) {
	s.previous = previous
}

// GetPrevious returns the previous state
func (s *State) GetPrevious() StateStatus {
	return s.previous
}

// SetCurrent sets the current state
func (s *State) SetCurrent(current StateStatus) {
	s.current = current
}

// GetCurrent returns the current state
func (s *State) GetCurrent() StateStatus {
	return s.current
}

// IsCurrentStateAFinalState returns true if the current state is a final state
func (s *State) IsCurrentStateAFinalState() bool {
	return s.current == StateStatusUP || s.current == StateStatusDOWN
}

// IsPreviousStateAFinalState returns true if the previous state is a final state
func (s *State) IsPreviousStateAFinalState() bool {
	return s.previous == StateStatusUP || s.previous == StateStatusDOWN
}

// IsCurrentStatusUP returns if the current status up
func (s *State) IsCurrentStatusUP() bool {
	if s.GetCurrent() == StateStatusUP || s.GetCurrent() == StateStatusINIT {
		return true
	}
	return false
}

// IsCurrentStatusDOWN returns if the current status down
func (s *State) IsCurrentStatusDOWN() bool {
	if s.GetCurrent() == StateStatusDOWN || s.GetCurrent() == StateStatusINIT {
		return true
	}
	return false
}

// IsPreviousStatusUP returns if the previous status ok
func (s *State) IsPreviousStatusUP() bool {
	if s.GetPrevious() == StateStatusUP || s.GetPrevious() == StateStatusINIT {
		return true
	}
	return false
}

// IsPreviousStatusDOWN returns if the previous status error
func (s *State) IsPreviousStatusDOWN() bool {
	if s.GetPrevious() == StateStatusDOWN || s.GetPrevious() == StateStatusINIT {
		return true
	}
	return false
}

// SetStateChangeTime sets the state change time
func (s *State) SetStateChangeTime(stateChangeTime time.Time) {
	s.stateChangeTime = stateChangeTime
}

// GetStateChangeTime returns the state change time
func (s *State) GetStateChangeTime() time.Time {
	return s.stateChangeTime
}

// Update updates the state of the monitor
func (s *State) Update(newState StateStatus) error {
	s.sMutex.Lock()
	defer s.sMutex.Unlock()
	// Validate newState
	if newState != StateStatusUP &&
		newState != StateStatusDOWN &&
		newState != StateStatusINIT {
		return fmt.Errorf("Invalid state: %s", newState)
	}
	s.SetPrevious(s.GetCurrent())
	s.SetCurrent(newState)
	s.SetStateChangeTime(time.Now())
	s.stateChan <- s
	return nil
}

// UnSubscribe unsubscribes the subscriber
func (s *State) UnSubscribe(id string) {
	s.sMutex.Lock()
	defer s.sMutex.Unlock()
	delete(s.subscribers, id)
	log.Debug("Unsubscribed from state updates for: ", id)
}

// Subscribe subscribes to state updates
func (s *State) Subscribe(id string, subscriber func(s *State)) {
	s.sMutex.Lock()
	defer s.sMutex.Unlock()
	s.subscribers[id] = subscriber
	log.Debug("Subscribed to state updates: ", id)
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
	monitorMu           sync.RWMutex
	monitors            = make(map[string]func(configBody []byte, logger Logger, metricsEnabled bool) (Monitor, error))
	initializedMonitors = make(map[string]Monitor)
)

// RegisterMonitor registers a monitor
func RegisterMonitor(monitorType MonitorType, initFunc func(configBody []byte, logger Logger, metricsEnabled bool) (Monitor, error)) error {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	if _, dup := monitors[monitorType.String()]; dup {
		return fmt.Errorf("monitor is already registered: %s", monitorType.String())
	}
	log.Info("Registering monitor: ", monitorType.String())
	monitors[monitorType.String()] = initFunc
	return nil
}

// GetMonitor returns a monitor
func GetMonitor(name string, configBody []byte, logger Logger, metricsEnabled bool) (Monitor, error) {
	monitorMu.RLock()
	defer monitorMu.RUnlock()
	if initFunc, ok := monitors[name]; ok {
		m, err := initFunc(configBody, logger, metricsEnabled)
		if err != nil {
			return nil, err
		}
		if initializedMonitors[m.Name().String()] != nil {
			m = initializedMonitors[m.Name().String()]
		} else {
			initializedMonitors[m.Name().String()] = m
		}
		return m, nil
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

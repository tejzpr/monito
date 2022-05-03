package monitors

import (
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/tejzpr/monito/log"
	"github.com/tejzpr/monito/types"
	"github.com/tejzpr/monito/utils"
)

var isStringAlphabetic = regexp.MustCompile(`^[a-zA-Z0-9_]*$`).MatchString
var symbolsRegexp = regexp.MustCompile(`[^\w]`)

// MonitorName is a string that represents the name of the monitor
type MonitorName string

func (m MonitorName) String() string {
	return symbolsRegexp.ReplaceAllString(string(m), "_")
}

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
	Name     MonitorName       `json:"name"`
	Type     types.MonitorType `json:"type"`
	EndPoint string            `json:"endPoint"`
	Time     time.Time         `json:"time"`
	Status   StateStatus       `json:"status"`
	Error    error             `json:"error,omitempty"`
}

// GetName returns the name of the monitor from NotificationBody
func (n *NotificationBody) GetName() MonitorName {
	return n.Name
}

// GetNameString returns the name of the monitor from NotificationBody
func (n *NotificationBody) GetNameString() string {
	return string(n.Name)
}

// GetType returns the type of the monitor from NotificationBody
func (n *NotificationBody) GetType() types.MonitorType {
	return n.Type
}

// GetTypeString returns the type of the monitor from NotificationBody
func (n *NotificationBody) GetTypeString() string {
	return string(n.Type)
}

// GetEndPoint returns the end point for the state from NotificationBody
func (n *NotificationBody) GetEndPoint() string {
	return n.EndPoint
}

// GetTime returns the time of the state change from NotificationBody
func (n *NotificationBody) GetTime() time.Time {
	return n.Time
}

// GetStatus returns the status of the state from NotificationBody
func (n *NotificationBody) GetStatus() StateStatus {
	return n.Status
}

// GetStatusString returns the status string for the state from NotificationBody
func (n *NotificationBody) GetStatusString() string {
	return string(n.Status)
}

// GetError returns the error for the state from NotificationBody
func (n *NotificationBody) GetError() error {
	return n.Error
}

// GetErrorString returns the error string for the state from NotificationBody
func (n *NotificationBody) GetErrorString() string {
	if n.Error != nil {
		return n.Error.Error()
	}
	return ""
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
	error           error
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
func (s *State) setPrevious(previous StateStatus) {
	s.previous = previous
}

// GetPrevious returns the previous state
func (s *State) GetPrevious() StateStatus {
	return s.previous
}

// SetCurrent sets the current state
func (s *State) setCurrent(current StateStatus) {
	s.current = current
}

// GetCurrent returns the current state
func (s *State) GetCurrent() StateStatus {
	return s.current
}

// setError sets the error for the state
func (s *State) setError(err error) {
	s.error = err
}

// GetError returns the error for the state
func (s *State) GetError() error {
	return s.error
}

// GetErrorString returns the error string for the state
func (s *State) GetErrorString() string {
	if s.error != nil {
		return s.error.Error()
	}
	return ""
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

// setStateChangeTime sets the state change time
func (s *State) setStateChangeTime(stateChangeTime time.Time) {
	s.stateChangeTime = stateChangeTime
}

// GetStateChangeTime returns the state change time
func (s *State) GetStateChangeTime() time.Time {
	return s.stateChangeTime
}

// Update updates the state of the monitor
func (s *State) Update(newState StateStatus, err error) error {
	s.sMutex.Lock()
	defer s.sMutex.Unlock()
	// Validate newState
	if newState != StateStatusUP &&
		newState != StateStatusDOWN &&
		newState != StateStatusINIT {
		return fmt.Errorf("Invalid state: %s", newState)
	}
	s.setPrevious(s.GetCurrent())
	s.setError(err)
	s.setCurrent(newState)
	s.setStateChangeTime(time.Now())
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
	monitors            = make(map[string]func(configBody []byte, logger Logger, jitterFactor int, prometheusMetricsEnabled bool) (Monitor, error))
	initializedMonitors = make(map[string]Monitor)
)

// RegisterMonitor takes a monitor type and a function that returns a monitor and adds it to a map of monitors
func RegisterMonitor(monitorType types.MonitorType, initFunc func(configBody []byte, logger Logger, jitterFactor int, prometheusMetricsEnabled bool) (Monitor, error)) error {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	if _, dup := monitors[monitorType.String()]; dup {
		return fmt.Errorf("monitor is already registered: %s", monitorType.String())
	}
	log.Info("Registering monitor: ", monitorType.String())
	monitors[monitorType.String()] = initFunc
	return nil
}

// GetMonitor takes a monitor name, a config body, a logger, and a boolean indicating whether metrics are
// enabled, and returns a monitor and an error
func GetMonitor(name string, configBody []byte, logger Logger, jitterFactor int, prometheusMetricsEnabled bool) (Monitor, error) {
	monitorMu.RLock()
	defer monitorMu.RUnlock()
	if initFunc, ok := monitors[name]; ok {
		m, err := initFunc(configBody, logger, jitterFactor, prometheusMetricsEnabled)
		if err != nil {
			return nil, err
		}
		if initializedMonitors[m.GetName().String()] != nil {
			m = initializedMonitors[m.GetName().String()]
		} else {
			initializedMonitors[m.GetName().String()] = m
		}
		return m, nil
	}
	return nil, fmt.Errorf("monitor is not registered: %s", name)
}

// CheckIfMonitorRegistered checks if a monitor is registered by name.
func CheckIfMonitorRegistered(name string) bool {
	monitorMu.RLock()
	defer monitorMu.RUnlock()
	_, ok := monitors[name]
	return ok
}

// GetRegisteredMonitorNames returns a slice of strings containing the names of all the registered monitors
func GetRegisteredMonitorNames() []string {
	monitorMu.RLock()
	defer monitorMu.RUnlock()
	var names []string
	for name := range monitors {
		names = append(names, name)
	}
	return names
}

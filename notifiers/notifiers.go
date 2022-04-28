package notifiers

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/tejzpr/monito/log"
	"github.com/tejzpr/monito/monitors"
)

var symbolsRegexp = regexp.MustCompile(`[^\w]`)

// NotifierName is a string that represents the name of the notifier
type NotifierName string

func (m NotifierName) String() string {
	return symbolsRegexp.ReplaceAllString(string(m), "_")
}

// Notifier is the interface for all notifiers
type Notifier interface {
	Notify(nBody *monitors.NotificationBody, params ...interface{}) error
	GetName() NotifierName
	Close() error
}

var (
	notifierMu          sync.RWMutex
	notifiers           = make(map[NotifierName]func(configBody []byte) (Notifier, error))
	initalizedNotifiers = make(map[NotifierName]Notifier)
)

// RegisterNotifier registers a notifier
func RegisterNotifier(name NotifierName, initFunc func(configBody []byte) (Notifier, error)) error {
	notifierMu.Lock()
	defer notifierMu.Unlock()
	if _, dup := notifiers[name]; dup {
		return fmt.Errorf("notifier is already registered: %s", name)
	}
	log.Info("Registering notifier: ", name)
	notifiers[name] = initFunc
	return nil
}

// InitNotifier returns the notifier with the given name
func InitNotifier(name NotifierName, configBody []byte) (Notifier, error) {
	notifierMu.RLock()
	defer notifierMu.RUnlock()
	if initFunc, ok := notifiers[name]; ok {
		n, err := initFunc(configBody)
		if err != nil {
			return nil, err
		}
		initalizedNotifiers[name] = n
		return n, nil
	}
	return nil, fmt.Errorf("monitor is not registered: %s", name)
}

// GetNotifier returns the notifier with the given name
func GetNotifier(name NotifierName) Notifier {
	notifierMu.RLock()
	defer notifierMu.RUnlock()
	if notifier, ok := initalizedNotifiers[name]; ok {
		return notifier
	}
	return nil
}

// StopAll stops all notifiers
func StopAll() {
	notifierMu.Lock()
	defer notifierMu.Unlock()
	for _, notifier := range initalizedNotifiers {
		notifier.Close()
	}
}

// GetRegisteredNotifierNames returns the registered monitor names
func GetRegisteredNotifierNames() []NotifierName {
	notifierMu.RLock()
	defer notifierMu.RUnlock()
	var names []NotifierName
	for name := range initalizedNotifiers {
		names = append(names, name)
	}
	return names
}

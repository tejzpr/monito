package notifiers

import (
	"fmt"
	"sync"
)

// Notifier is the interface for all notifiers
type Notifier interface {
	Notify(params ...interface{}) error
	GetName() string
	Configure(params ...interface{}) error
	Close() error
}

var (
	notifierMu sync.RWMutex
	notifiers  = make(map[string]Notifier)
)

// RegisterNotifier registers a notifier
func RegisterNotifier(name string, params ...interface{}) error {
	notifierMu.Lock()
	defer notifierMu.Unlock()
	if _, dup := notifiers[name]; dup {
		return fmt.Errorf("notifier is already registered: %s", name)
	}
	switch name {
	case "webex":
		w := &WebexNotifier{}
		if err := w.Configure(params...); err != nil {
			return err
		}
		notifiers[name] = w
	case "smtp":
		s := &SMTPNotifier{}
		if err := s.Configure(params...); err != nil {
			return err
		}
		notifiers[name] = s
	default:
		return fmt.Errorf("unknown notifier %s", name)
	}
	return nil
}

// GetNotifier returns the notifier with the given name
func GetNotifier(name string) (Notifier, error) {
	if notifier, ok := notifiers[name]; ok {
		return notifier, nil
	}
	return nil, fmt.Errorf("notifier %s is not registered", name)
}

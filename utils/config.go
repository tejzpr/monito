package utils

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tejzpr/monito/notifiers/smtp"
	"github.com/tejzpr/monito/notifiers/webex"
)

// Duration is a wrapper for time.Duration
type Duration struct {
	time.Duration
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (duration *Duration) UnmarshalJSON(b []byte) error {
	var unmarshalledJSON interface{}

	err := json.Unmarshal(b, &unmarshalledJSON)
	if err != nil {
		return err
	}

	switch value := unmarshalledJSON.(type) {
	case float64:
		duration.Duration = time.Duration(value)
	case string:
		duration.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid duration: %#v", unmarshalledJSON)
	}

	return nil
}

// NotifyConfig is the config for the Notify notifier
type NotifyConfig struct {
	Webex webex.SendConfig `json:"webex"`
	SMTP  smtp.SendConfig  `json:"smtp"`
}

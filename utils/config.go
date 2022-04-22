package utils

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tejzpr/monito/monitors"
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

// HTTPConfigHeader is the header for the HTTP config
type HTTPConfigHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// WebexSendConfig is the config for the Webex notifier
type WebexSendConfig struct {
	RoomID string `json:"roomId"`
}

// SMTPSendConfig is the config for the Webex notifier
type SMTPSendConfig struct {
	To  []string `json:"to"`
	Cc  []string `json:"cc"`
	Bcc []string `json:"bcc"`
}

// NotifyConfig is the config for the Notify notifier
type NotifyConfig struct {
	Webex WebexSendConfig `json:"webex"`
	SMTP  SMTPSendConfig  `json:"smtp"`
}

// HTTPConfig is the config for the HTTP monitor
type HTTPConfig struct {
	Name                  monitors.MonitorName `json:"name"`
	Type                  string               `json:"type"`
	URL                   string               `json:"url"`
	Method                string               `json:"method"`
	Headers               HTTPConfigHeader     `json:"headers"`
	ExpectedStatusCode    int                  `json:"expectedStatusCode"`
	ExpectedResponseBody  string               `json:"expectedResponseBody"`
	Interval              Duration             `json:"interval"`
	Timeout               Duration             `json:"timeout"`
	MaxConcurrentRequests int                  `json:"maxConcurrentRequests"`
	MaxRetries            int                  `json:"maxRetries"`
	NotifyRateLimit       Duration             `json:"notifyRateLimit"`
	NotifyDetails         NotifyConfig         `json:"notifyDetails"`
}

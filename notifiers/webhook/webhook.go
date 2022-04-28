package smtp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/tejzpr/monito/log"
	"github.com/tejzpr/monito/monitors"
	"github.com/tejzpr/monito/notifiers"
)

func init() {
	notifiers.RegisterNotifier("webhook", InitWebHookNotifier)
}

// SendConfig is the config for the notifier
type SendConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// Notifier is the notifier for email
type Notifier struct {
	url     string
	headers map[string]string
	enabled bool
	client  *http.Client
}

type WebhookResponse struct {
	Subject string `json:"subject"`
	Message string `json:"message"`
}

type webhookNotificationBody struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	EndPoint string `json:"endPoint"`
	Time     string `json:"time"`
	Status   string `json:"status"`
}

// Notify sends the message by email
// params[0] is the Mail object
func (s *Notifier) Notify(nBody *monitors.NotificationBody, params ...interface{}) error {
	err := s.validate()
	if err != nil {
		return err
	}

	jBytes := params[0].([]byte)
	var sConfig SendConfig
	if err := json.Unmarshal(jBytes, &sConfig); err != nil {
		log.Errorf(err, "Error unmarshalling notifier")
		return err
	}

	msgBody := webhookNotificationBody{
		Name:     nBody.Name.String(),
		Type:     nBody.Type.String(),
		EndPoint: nBody.EndPoint,
		Time:     nBody.Time.Format(time.RFC1123),
		Status:   nBody.Status.String(),
	}

	var jsonStr = []byte{}
	jsonStr, err = json.Marshal(msgBody)
	if err != nil {
		log.Errorf(err, "Error marshalling WebhookResponse")
		return err
	}

	// Data
	w, err := s.client.Post(sConfig.URL, "application/json", bytes.NewBuffer(jsonStr))

	if err != nil {
		return err
	} else if w.StatusCode < 200 || w.StatusCode > 299 {
		return fmt.Errorf("HTTP status code: %d", w.StatusCode)
	}
	err = w.Body.Close()
	if err != nil {
		return err
	}
	return nil
}

// GetName returns the name of the notifier
func (s *Notifier) GetName() notifiers.NotifierName {
	return notifiers.NotifierName("email")
}

func (s *Notifier) connect() error {
	var netTransport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	s.client = &http.Client{
		Timeout:   time.Second * 10,
		Transport: netTransport,
	}

	return nil
}

// NotifierConfig is the config for the SMTP notifier
type NotifierConfig struct {
	Enabled bool `json:"enabled"`
}

// Validate validates the config
func (s *NotifierConfig) Validate() error {
	if !s.Enabled {
		return fmt.Errorf("disabled")
	}
	return nil
}

// Configure configures the notifier
func (s *Notifier) Configure(webhookConfig NotifierConfig) error {
	if !webhookConfig.Enabled {
		s.enabled = false
		return fmt.Errorf("disabled")
	}
	s.enabled = true
	return s.connect()
}

// Close closes the notifier
func (s *Notifier) Close() error {
	s.client.CloseIdleConnections()
	return nil
}

// Validate validates the notifier
func (s *Notifier) validate() error {
	return nil
}

var webhookInstance *Notifier
var webhookOnce sync.Once

// InitWebHookNotifier initializes the smtp notifier
func InitWebHookNotifier(configBody []byte) (notifiers.Notifier, error) {
	var mConfig NotifierConfig
	if err := json.Unmarshal(configBody, &mConfig); err != nil {
		log.Errorf(err, "Error unmarshalling config for notifier: webhook")
		return nil, err
	}
	err := mConfig.Validate()
	if err != nil {
		return nil, err
	}

	webhookOnce.Do(func() {
		webhookInstance = &Notifier{}
		err = webhookInstance.Configure(mConfig)
	})
	return webhookInstance, err
}

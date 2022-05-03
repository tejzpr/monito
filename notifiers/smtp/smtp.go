package smtp

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/tejzpr/monito/log"
	"github.com/tejzpr/monito/monitors"
	"github.com/tejzpr/monito/notifiers"
)

var notifierName notifiers.NotifierName = "smtp"

func init() {
	notifiers.RegisterNotifier(notifierName, InitSMTPNotifier)
}

// SendConfig is the config for the smtp notifier
type SendConfig struct {
	To  []string `json:"to"`
	Cc  []string `json:"cc"`
	Bcc []string `json:"bcc"`
}

// BuildMessage builds the message
func (mail *SendConfig) BuildMessage(sender string, subject string, body string) string {
	header := ""
	header += fmt.Sprintf("From: %s\r\n", sender)
	if len(mail.To) > 0 {
		header += fmt.Sprintf("To: %s\r\n", strings.Join(mail.To, ";"))
	}
	if len(mail.Cc) > 0 {
		header += fmt.Sprintf("Cc: %s\r\n", strings.Join(mail.Cc, ";"))
	}

	header += fmt.Sprintf("Subject: %s\r\n", subject)
	header += "\r\n" + body

	return header
}

// Notifier is the notifier for email
type Notifier struct {
	host      string
	port      string
	sender    string
	enabled   bool
	tls       bool
	client    *smtp.Client
	tlsConfig *tls.Config
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

	if len(sConfig.To) <= 0 {
		return fmt.Errorf("no recipients")
	}

	subject := fmt.Sprintf("%s: %s", nBody.Name, nBody.Status)
	message := fmt.Sprintf("Name: %s\nType: %s\nEndpoint: %s\nStatus: %s\nDetails: %s\n\nTime: %s\n", nBody.Name, nBody.Type, nBody.EndPoint, nBody.Status, nBody.GetErrorString(), nBody.Time.Format(time.RFC1123))

	messageBody := sConfig.BuildMessage(s.sender, subject, message)
	if err := s.client.Mail(s.sender); err != nil {
		return err
	}
	receivers := append(sConfig.To, sConfig.Cc...)
	receivers = append(receivers, sConfig.Bcc...)
	for _, k := range receivers {
		if err := s.client.Rcpt(k); err != nil {
			return err
		}
	}

	// Data
	w, err := s.client.Data()
	if err != nil {
		return err
	}

	_, err = w.Write([]byte(messageBody))
	if err != nil {
		return err
	}

	err = w.Close()
	if err != nil {
		return err
	}
	return nil
}

func (s *Notifier) serverName() string {
	return s.host + ":" + s.port
}

// GetName returns the name of the notifier
func (s *Notifier) GetName() notifiers.NotifierName {
	return notifierName
}

func (s *Notifier) connect() error {
	if s.tls {
		s.tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         s.host,
		}
		conn, err := tls.Dial("tcp", s.serverName(), s.tlsConfig)
		if err != nil {
			return err
		}

		client, err := smtp.NewClient(conn, s.host)
		if err != nil {
			return err
		}

		s.client = client
	} else {
		client, err := smtp.Dial(s.serverName())
		if err != nil {
			return err
		}
		s.client = client
	}
	return nil
}

// NotifierConfig is the config for the SMTP notifier
type NotifierConfig struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	TLS     bool   `json:"tls"`
	Sender  string `json:"sender"`
}

// Validate validates the config
func (s *NotifierConfig) Validate() error {
	if !s.Enabled {
		return fmt.Errorf("disabled")
	}
	if s.Host == "" {
		return fmt.Errorf("host is required")
	}
	if s.Port == 0 {
		return fmt.Errorf("port is required")
	}
	if s.Sender == "" {
		return fmt.Errorf("sender is required")
	}
	return nil
}

// Configure configures the notifier
func (s *Notifier) Configure(smtpConfig NotifierConfig) error {
	if !smtpConfig.Enabled {
		s.enabled = false
		return fmt.Errorf("disabled")
	}
	s.enabled = true
	s.host = smtpConfig.Host
	s.tls = smtpConfig.TLS
	s.port = fmt.Sprintf("%d", smtpConfig.Port)
	s.sender = smtpConfig.Sender
	return s.connect()
}

// Close closes the notifier
func (s *Notifier) Close() error {
	return s.client.Quit()
}

// Validate validates the notifier
func (s *Notifier) validate() error {
	if s.client.Noop() != nil {
		return s.connect()
	}
	return nil
}

var smtpInstance *Notifier
var smtpOnce sync.Once

// InitSMTPNotifier initializes the smtp notifier
func InitSMTPNotifier(configBody []byte) (notifiers.Notifier, error) {
	var mConfig NotifierConfig
	if err := json.Unmarshal(configBody, &mConfig); err != nil {
		log.Errorf(err, "Error unmarshalling config for notifier: smtp")
		return nil, err
	}
	err := mConfig.Validate()
	if err != nil {
		return nil, err
	}

	smtpOnce.Do(func() {
		smtpInstance = &Notifier{}
		err = smtpInstance.Configure(mConfig)
	})
	return smtpInstance, err
}

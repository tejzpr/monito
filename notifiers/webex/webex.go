package webex

import (
	"encoding/json"
	"fmt"
	"sync"

	webexteams "github.com/jbogarin/go-cisco-webex-teams/sdk"
	"github.com/tejzpr/monito/log"
	"github.com/tejzpr/monito/notifiers"
)

func init() {
	notifiers.RegisterNotifier("webex", InitWebexNotifier)
}

// SendConfig is the config for the Webex notifier
type SendConfig struct {
	RoomID string `json:"roomId"`
}

// Notifier is the notifier for webex
type Notifier struct {
	enabled        bool
	accessToken    string
	roomID         string
	client         *webexteams.Client
	connectMutex   sync.Mutex
	roomIDVerified bool
}

// Notify sends the message to webex
// params[0] is the message
// params[1] is the room id
func (w *Notifier) Notify(subject string, message string, params ...interface{}) error {
	if w.client == nil {
		return fmt.Errorf("webex client is not configured")
	}

	jBytes := params[0].([]byte)
	var sConfig SendConfig
	if err := json.Unmarshal(jBytes, &sConfig); err != nil {
		log.Errorf(err, "Error unmarshalling notifier")
		return err
	}

	if len(sConfig.RoomID) <= 0 {
		return fmt.Errorf("room id is not configured")
	} else if len(message) <= 0 {
		return fmt.Errorf("message is not configured")
	}

	_, _, err := w.client.Messages.CreateMessage(&webexteams.MessageCreateRequest{
		RoomID:   sConfig.RoomID,
		Markdown: message,
	})
	if err != nil {
		return err
	}
	return nil
}

// GetName returns the name of the notifier
func (w *Notifier) GetName() notifiers.NotifierName {
	return notifiers.NotifierName("webex")
}

func (w *Notifier) connect() error {
	w.connectMutex.Lock()
	defer w.connectMutex.Unlock()
	w.client = webexteams.NewClient()
	w.client.SetAuthToken(w.accessToken)
	me, _, err := w.client.People.GetMe()
	if err != nil {
		return fmt.Errorf("webex: unable to verify token %s", err.Error())
	} else if me.ID == "" {
		return fmt.Errorf("webex: unable to verify token")
	}
	return nil
}

// NotifierConfig is the config for the Webex notifier
type NotifierConfig struct {
	Enabled     bool   `json:"enabled"`
	AccessToken string `json:"accessToken"`
}

// Validate validates the config
func (w *NotifierConfig) Validate() error {
	if !w.Enabled {
		return fmt.Errorf("disabled")
	}
	if w.AccessToken == "" {
		return fmt.Errorf("accessToken is required")
	}
	return nil
}

// Configure configures the notifier
func (w *Notifier) Configure(webexConfig NotifierConfig) error {
	if !webexConfig.Enabled {
		w.enabled = false
		return fmt.Errorf("disabled")
	}
	w.enabled = true
	accessToken := webexConfig.AccessToken
	if w.client == nil {
		w.accessToken = accessToken
		err := w.connect()
		if err != nil {
			return err
		}
	}
	return nil
}

// Close closes the notifier
func (w *Notifier) Close() error {
	return nil
}

var webexInstance *Notifier
var webexOnce sync.Once

// InitWebexNotifier initializes the webex notifier
func InitWebexNotifier(configBody []byte) (notifiers.Notifier, error) {
	var mConfig NotifierConfig
	if err := json.Unmarshal(configBody, &mConfig); err != nil {
		log.Errorf(err, "Error unmarshalling config for notifier: smtp")
		return nil, err
	}
	err := mConfig.Validate()
	if err != nil {
		return nil, err
	}

	webexOnce.Do(func() {
		webexInstance = &Notifier{}
		err = webexInstance.Configure(mConfig)
	})
	return webexInstance, err
}

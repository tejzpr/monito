package notifiers

import (
	"encoding/json"
	"fmt"
	"sync"

	webexteams "github.com/jbogarin/go-cisco-webex-teams/sdk"
)

// WebexNotifier is the notifier for webex
type WebexNotifier struct {
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
func (w *WebexNotifier) Notify(params ...interface{}) error {
	if w.client == nil {
		return fmt.Errorf("webex client is not configured")
	}
	message := params[0].(string)
	roomID := params[1].(string)

	_, _, err := w.client.Messages.CreateMessage(&webexteams.MessageCreateRequest{
		RoomID:   roomID,
		Markdown: message,
	})
	if err != nil {
		return err
	}
	return nil
}

// GetName returns the name of the notifier
func (w *WebexNotifier) GetName() string {
	return "webex"
}

func (w *WebexNotifier) connect() error {
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

// WebexNotifierConfig is the config for the Webex notifier
type WebexNotifierConfig struct {
	Enabled     bool   `json:"enabled"`
	AccessToken string `json:"accessToken"`
}

// Configure configures the notifier
// params[0] is map[string]interface{} of type  WebexNotifierConfig
func (w *WebexNotifier) Configure(params ...interface{}) error {
	config := params[0].(map[string]interface{})
	jsonBody, err := json.Marshal(config)
	if err != nil {
		return err
	}
	var webexConfig WebexNotifierConfig
	if err := json.Unmarshal(jsonBody, &webexConfig); err != nil {
		return err
	}

	if !webexConfig.Enabled {
		w.enabled = false
		return nil
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
func (w *WebexNotifier) Close() error {
	return nil
}

var webexInstance *WebexNotifier
var webexOnce sync.Once

// InitWebexNotifier initializes the webex notifier
func InitWebexNotifier(accessToken string) (*WebexNotifier, error) {
	var err error
	webexOnce.Do(func() {
		webexInstance = &WebexNotifier{}
		err = webexInstance.Configure(accessToken)
	})
	return webexInstance, err
}

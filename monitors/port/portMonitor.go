package port

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tejzpr/monito/log"
	"github.com/tejzpr/monito/monitors"
	"github.com/tejzpr/monito/types"
)

// NetworkProtocol returns the network protocol
type NetworkProtocol string

const (
	// TCP is the tcp protocol
	TCP NetworkProtocol = "tcp"
	// UDP is the udp protocol
	UDP NetworkProtocol = "udp"
)

// GetProtocol returns the protocol
// Defaults to tcp
func (n NetworkProtocol) GetProtocol(p string) NetworkProtocol {
	p = strings.ToLower(p)
	if p == "tcp" {
		return TCP
	} else if p == "udp" {
		return UDP
	}
	return TCP
}

// String returns the string representation of the protocol
func (n NetworkProtocol) String() string {
	return string(n)
}

// Config is the config for the Port monitor
type Config struct {
	// Host is the host to monitor
	Host string `json:"host"`
	// Port is the Port to monitor
	Port uint64 `json:"port"`
	// Protocol is the protocol to use
	Protocol NetworkProtocol `json:"protocol"`
}

// Monitor is a monitor that monitors ports
// it implements the Monitor interface
type Monitor struct {
	monitors.BaseMonitor
	Config *Config
}

// Init initializes the monitor
func (m *Monitor) Init() error {
	err := m.Initialize()
	if err != nil {
		return err
	}
	return nil
}

// CheckPort checks if a port is open
func (m *Monitor) CheckPort() error {
	conn, err := net.DialTimeout(m.Config.Protocol.String(), net.JoinHostPort(m.Config.Host, strconv.FormatUint(m.Config.Port, 10)), m.TimeOut)
	if err != nil {
		return err
	}
	if conn != nil {
		defer conn.Close()
		return nil
	}
	return nil
}

// Process runs the monitor
func (m *Monitor) Process() error {
	defer m.Semaphore.Release(1)

	m.Logger.Debugf("Running Port Request for monitor: %s", m.Name)

	err := m.CheckPort()
	if err != nil {
		switch t := err.(type) {
		case *net.OpError:
			if t.Op == "dial" {
				return m.HandleFailure(errors.New("Unknown host"))
			} else if t.Op == "read" {
				return m.HandleFailure(errors.New("Connection refused [read]"))
			}
			return err
		case *url.Error:
			if t.Op == "Get" || t.Op == "Post" || t.Op == "Option" {
				return m.HandleFailure(errors.New("Connection refused [url error]"))
			}
			return err
		case syscall.Errno:
			if t == syscall.ECONNREFUSED {
				return m.HandleFailure(errors.New("Connection refused [ECONNREFUSED]"))
			}
			return err
		default:
			return err
		}
	}

	return nil
}

// GetConfig returns the config for the monitor
func (m *Monitor) GetConfig() interface{} {
	return m.Config
}

// SetConfig sets the config for the monitor
func (m *Monitor) SetConfig(config interface{}) error {
	conf := config.(*Config)
	if conf.Host == "" {
		return errors.New("Host is required")
	} else if conf.Protocol.String() == "" {
		return errors.New("Protocol is required")
	} else if conf.Port == 0 {
		return errors.New("Port is required")
	}
	m.Config = conf
	return nil
}

// GetNotificationBody returns the notification body
func (m *Monitor) GetNotificationBody(state *monitors.State) *monitors.NotificationBody {
	loc, _ := time.LoadLocation("UTC")
	now := time.Now()
	return &monitors.NotificationBody{
		Name:     m.GetName(),
		Type:     m.GetType(),
		EndPoint: fmt.Sprintf("%s://%s:%d", m.Config.Protocol.String(), m.Config.Host, m.Config.Port),
		Time:     now.In(loc),
		Status:   state.GetCurrent(),
		Error:    state.GetError(),
	}
}

// JSONConfig is the config for the Ping monitor
type JSONConfig struct {
	monitors.JSONBaseConfig
	Host     string          `json:"host"`
	Port     uint64          `json:"port"`
	Protocol NetworkProtocol `json:"protocol"`
}

// Validate validates the config for the Port monitor
func (m *JSONConfig) monitorFieldsValidate() error {
	if m.Host == "" {
		return errors.New("Host is required")
	} else if m.Protocol.String() == "" {
		return errors.New("Protocol is required")
	} else if m.Port == 0 {
		return errors.New("Port is required")
	}
	return nil
}

// newMonitor creates a new Port monitor
func newMonitor(configBody []byte, logger monitors.Logger, jitterFactor int, prometheusMetricsEnabled bool) (monitors.Monitor, error) {
	var mConfig JSONConfig
	if err := json.Unmarshal(configBody, &mConfig); err != nil {
		log.Errorf(err, "Error unmarshalling config for monitor: http")
		return nil, err
	}
	err := mConfig.Validate()
	if err != nil {
		return nil, err
	}
	err = mConfig.monitorFieldsValidate()
	if err != nil {
		return nil, err
	}
	portConfig := &Config{
		Host:     mConfig.Host,
		Port:     mConfig.Port,
		Protocol: mConfig.Protocol,
	}

	portMonitor := &Monitor{}
	portMonitor.SetName(mConfig.Name)
	err = portMonitor.SetConfig(portConfig)
	if err != nil {
		return nil, err
	}
	portMonitor.SetProcess(portMonitor.Process)
	portMonitor.SetDescription(mConfig.Description)
	portMonitor.SetGroup(mConfig.Group)
	portMonitor.SetLogger(logger)
	portMonitor.SetEnabled(mConfig.Enabled)
	portMonitor.SetInterval(mConfig.Interval.Duration)
	portMonitor.SetTimeOut(mConfig.Timeout.Duration)
	portMonitor.SetMaxConcurrentRequests(mConfig.MaxConcurrentRequests)
	portMonitor.SetMaxRetries(mConfig.MaxRetries)
	portMonitor.SetNotifyRateLimit(mConfig.NotifyRateLimit.Duration)
	portMonitor.SetJitterFactor(jitterFactor)
	portMonitor.SetEnablePrometheusMetrics(prometheusMetricsEnabled)
	portMonitor.SetType(types.MonitorType("port"))
	return portMonitor, nil
}

func init() {
	err := monitors.RegisterMonitor(types.MonitorType("port"), newMonitor)
	if err != nil {
		panic(err)
	}
}

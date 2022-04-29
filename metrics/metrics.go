package metrics

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tejzpr/monito/monitors"
)

// Metrics the metrics for the monitor
type Metrics struct {
	ServiceStatusGauge prometheus.Gauge
	LatencyGauge       prometheus.Gauge
	LatencyHistogram   prometheus.Histogram
}

// StartServiceStatusGauge initializes the service status gauge
func (hm *Metrics) StartServiceStatusGauge(name string, group string, monitorName monitors.MonitorType) {
	if group != "" {
		name = fmt.Sprintf("%s_%s", group, name)
	}
	if monitorName.String() == "" {
		monitorName = monitors.MonitorType("generic")
	}
	hm.ServiceStatusGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "monito",
		Subsystem: monitorName.String() + "_metrics",
		Name:      "is_service_up_" + name,
		Help:      "Provides status of the service, 0 = down, 1 = up",
	})
}

// StartLatencyGauge initializes the latency histogram
func (hm *Metrics) StartLatencyGauge(name string, group string, monitorName monitors.MonitorType) {
	if group != "" {
		name = fmt.Sprintf("%s_%s", group, name)
	}
	if monitorName.String() == "" {
		monitorName = monitors.MonitorType("generic")
	}
	hm.LatencyGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "monito",
		Subsystem: monitorName.String() + "_metrics",
		Name:      "latency_gauge_" + name,
		Help:      "Provides a gauge of the latency of successful requests",
	})
}

// RecordLatency records the latency
func (hm *Metrics) RecordLatency(latency time.Duration) {
	if hm != nil && hm.LatencyGauge != nil {
		hm.LatencyGauge.Set(latency.Seconds())
	}
}

// ServiceDown handles the service down
func (hm *Metrics) ServiceDown() {
	if hm != nil && hm.ServiceStatusGauge != nil {
		hm.ServiceStatusGauge.Set(0)
	}
}

// ServiceUp handles the service down
func (hm *Metrics) ServiceUp() {
	if hm != nil && hm.ServiceStatusGauge != nil {
		hm.ServiceStatusGauge.Set(1)
	}
}

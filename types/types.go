package types

import "regexp"

// MonitorType is a string that represents the type of the monitor
type MonitorType string

var symbolsRegexp = regexp.MustCompile(`[^\w]`)

func (m MonitorType) String() string {
	return symbolsRegexp.ReplaceAllString(string(m), "_")
}

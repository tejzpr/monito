package log

/**
 * @author Tejus Pratap
 */

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

var logger *MonitoLogger
var loggerOnce sync.Once
var zlog zerolog.Logger

type LogLevel int8

const (
	// DebugLevel defines debug log level.
	DebugLevel LogLevel = iota
	// InfoLevel defines info log level.
	InfoLevel
	// WarnLevel defines warn log level.
	WarnLevel
	// ErrorLevel defines error log level.
	ErrorLevel
	// FatalLevel defines fatal log level.
	FatalLevel
	// PanicLevel defines panic log level.
	PanicLevel
	// NoLevel defines an absent log level.
	NoLevel
	// Disabled disables the logger.
	Disabled

	// TraceLevel defines trace log level.
	TraceLevel LogLevel = -1
	// Values less than TraceLevel are handled as numbers.
)

func parseInterface(msgs []interface{}) string {
	var msg string
	for _, m := range msgs {
		msg += fmt.Sprintf("%v", m)
	}
	return msg
}

// MonitoLogger is a custom app logger
type MonitoLogger struct {
	zlog zerolog.Logger
}

// Logger returns the logger
func Logger() *MonitoLogger {
	loggerOnce.Do(func() {
		zerolog.TimeFieldFormat = time.RFC3339
		zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
		zlog = zerolog.New(os.Stdout).With().Timestamp().Logger()
		logger = &MonitoLogger{zlog: zlog}
	})
	return logger
}

// LogRoundTrip logs the request and response
func (l *MonitoLogger) LogRoundTrip(
	req *http.Request,
	res *http.Response,
	err error,
	start time.Time,
	dur time.Duration,
) error {
	var (
		e *zerolog.Event
	)

	// Set error level.
	//
	switch {
	case err != nil:
		e = l.zlog.Error()
	case res != nil && res.StatusCode > 0 && res.StatusCode < 300:
		e = l.zlog.Info()
	case res != nil && res.StatusCode > 299 && res.StatusCode < 500:
		e = l.zlog.Warn()
	case res != nil && res.StatusCode > 499:
		e = l.zlog.Error()
	default:
		e = l.zlog.Error()
	}

	// Log event.
	//
	if l.zlog.GetLevel() == zerolog.DebugLevel || l.zlog.GetLevel() == zerolog.TraceLevel {
		e.Str("method", req.Method).
			Int("status_code", res.StatusCode).
			Dur("duration", dur).
			Int64("req_bytes", req.ContentLength).
			Int64("res_bytes", res.ContentLength).
			Msg(req.URL.String())
	}
	return nil
}

// RequestBodyEnabled makes the client pass request body to logger
func (l *MonitoLogger) RequestBodyEnabled() bool { return false }

// RequestBodyEnabled makes the client pass response body to logger
func (l *MonitoLogger) ResponseBodyEnabled() bool { return false }

// ZLogger returns the zerolog logger
func ZLogger() zerolog.Logger {
	return zlog
}

// SetLogLevel sets the log level
func SetLogLevel(logLevel string) {
	switch strings.ToLower(logLevel) {
	case "debug":
		zlog.Info().Stack().Msg("Setting log level to debug")
		zlog = zlog.Level(zerolog.DebugLevel)
		logger = &MonitoLogger{zlog}
		break
	case "trace":
		zlog.Info().Stack().Msg("Setting log level to trace")
		zlog = zlog.Level(zerolog.TraceLevel)
		logger = &MonitoLogger{zlog}
		break
	default:
		zlog.Info().Stack().Msg("Setting log level to info")
		zlog = zlog.Level(zerolog.InfoLevel)
		logger = &MonitoLogger{zlog}
		break
	}
}

// GetLevel returns the log level
func (l *MonitoLogger) GetLevel() LogLevel {
	return LogLevel(l.zlog.GetLevel())
}

// SetLevel sets the log level
func (l *MonitoLogger) SetLevel(level LogLevel) {
	l.zlog.Level(zerolog.Level(level))
}

// Info logs an info message
func (l *MonitoLogger) Info(msg ...interface{}) {
	l.zlog.Info().Stack().Msg(parseInterface(msg))
}

// Infof logs an info formatted string
func (l *MonitoLogger) Infof(format string, a ...interface{}) {
	l.zlog.Info().Stack().Msgf(format, a...)
}

// Debug logs an debug message
func (l *MonitoLogger) Debug(msg ...interface{}) {
	if LogLevel(l.zlog.GetLevel()) == DebugLevel {
		l.zlog.Debug().Stack().Msg(parseInterface(msg))
	}
}

// Debugf logs an debug formatted string
func (l *MonitoLogger) Debugf(format string, a ...interface{}) {
	if LogLevel(l.zlog.GetLevel()) == DebugLevel {
		l.zlog.Debug().Stack().Msgf(format, a...)
	}
}

// Warn logs a fatal message
func (l *MonitoLogger) Warn(msg ...interface{}) {
	l.zlog.Warn().Stack().Msg(parseInterface(msg))
}

// Warnf logs a fatal message
func (l *MonitoLogger) Warnf(format string, v ...interface{}) {
	l.zlog.Warn().Stack().Msgf(format, v...)
}

// Error logs an error with the message
func (l *MonitoLogger) Error(err error, msg ...interface{}) {
	if msg != nil && len(msg) > 0 {
		l.zlog.Error().Stack().Err(err).Msg(parseInterface(msg))
	} else {
		l.zlog.Error().Stack().Err(err).Msg(err.Error())
	}
}

// Errorf logs an error with the formatted string
func (l *MonitoLogger) Errorf(err error, format string, a ...interface{}) {
	if len(format) > 0 {
		l.zlog.Error().Stack().Err(err).Msgf(format, a...)
	} else {
		l.zlog.Error().Stack().Err(err).Msg(err.Error())
	}
}

// ErrorO logs an error with the message
func (l *MonitoLogger) ErrorO(err error, msg string) {
	l.zlog.Error().Stack().Err(err).Msg(msg)
}

// ErrorOf logs an error with the formatted string
func (l *MonitoLogger) ErrorOf(err error, format string, a ...interface{}) {
	l.zlog.Error().Stack().Err(err).Msgf(format, a...)
}

// Print logs a message
func (l *MonitoLogger) Print(v ...interface{}) {
	l.zlog.Print(v...)
}

// Printf logs a formatted string
func (l *MonitoLogger) Printf(format string, v ...interface{}) {
	l.zlog.Printf(format, v...)
}

// Fatal logs a fatal message
func (l *MonitoLogger) Fatal(msg ...interface{}) {
	l.zlog.Fatal().Stack().Msg(parseInterface(msg))
}

// Fatalf logs a fatal message
func (l *MonitoLogger) Fatalf(format string, v ...interface{}) {
	l.zlog.Fatal().Stack().Msgf(format, v...)
}

// Trace logs a fatal message
func (l *MonitoLogger) Trace(msg ...interface{}) {
	if LogLevel(l.zlog.GetLevel()) <= DebugLevel {
		l.zlog.Trace().Stack().Msg(parseInterface(msg))
	}
}

// Tracef logs a fatal message
func (l *MonitoLogger) Tracef(format string, v ...interface{}) {
	if LogLevel(l.zlog.GetLevel()) <= DebugLevel {
		l.zlog.Trace().Stack().Msgf(format, v...)
	}
}

// Panic logs a fatal message
func (l *MonitoLogger) Panic(msg ...interface{}) {
	l.zlog.Panic().Stack().Msg(parseInterface(msg))
}

// Panicf logs a fatal message
func (l *MonitoLogger) Panicf(format string, v ...interface{}) {
	l.zlog.Panic().Stack().Msgf(format, v...)
}

// Level returns the current log level
func (l *MonitoLogger) Level() LogLevel {
	return LogLevel(l.zlog.GetLevel())
}

// Info logs an info message
func Info(msg ...interface{}) {
	Logger().zlog.Info().Stack().Msg(parseInterface(msg))
}

// Infof logs an info formatted string
func Infof(format string, a ...interface{}) {
	Logger().zlog.Info().Stack().Msgf(format, a...)
}

// Debug logs an debug message
func Debug(msg ...interface{}) {
	if LogLevel(Logger().zlog.GetLevel()) == DebugLevel {
		Logger().zlog.Debug().Stack().Msg(parseInterface(msg))
	}
}

// Debugf logs an debug formatted string
func Debugf(format string, a ...interface{}) {
	if LogLevel(Logger().zlog.GetLevel()) == DebugLevel {
		Logger().zlog.Debug().Stack().Msgf(format, a...)
	}
}

// Warn logs a fatal message
func Warn(msg ...interface{}) {
	Logger().zlog.Warn().Stack().Msg(parseInterface(msg))
}

// Warnf logs a fatal message
func Warnf(format string, v ...interface{}) {
	Logger().zlog.Warn().Stack().Msgf(format, v...)
}

// Error logs an error with the message
func Error(err error, msg ...interface{}) {
	if msg != nil && len(msg) > 0 {
		Logger().zlog.Error().Stack().Err(err).Msg(parseInterface(msg))
	} else {
		Logger().zlog.Error().Stack().Err(err).Msg(err.Error())
	}
}

// Errorf logs an error with the formatted string
func Errorf(err error, format string, a ...interface{}) {
	if len(format) > 0 {
		Logger().zlog.Error().Stack().Err(err).Msgf(format, a...)
	} else {
		Logger().zlog.Error().Stack().Err(err).Msg(err.Error())
	}
}

// ErrorO logs an error with the message
func ErrorO(err error, msg string) {
	Logger().zlog.Error().Stack().Err(err).Msg(msg)
}

// ErrorOf logs an error with the formatted string
func ErrorOf(err error, format string, a ...interface{}) {
	Logger().zlog.Error().Stack().Err(err).Msgf(format, a...)
}

// Print logs a message
func Print(v ...interface{}) {
	Logger().zlog.Print(v...)
}

// Printf logs a formatted string
func Printf(format string, v ...interface{}) {
	Logger().zlog.Printf(format, v...)
}

// Fatal logs a fatal message
func Fatal(msg ...interface{}) {
	Logger().zlog.Fatal().Stack().Msg(parseInterface(msg))
}

// Fatalf logs a fatal message
func Fatalf(format string, v ...interface{}) {
	Logger().zlog.Fatal().Stack().Msgf(format, v...)
}

// Trace logs a fatal message
func Trace(msg ...interface{}) {
	if LogLevel(Logger().zlog.GetLevel()) <= DebugLevel {
		Logger().zlog.Trace().Stack().Msg(parseInterface(msg))
	}
}

// Tracef logs a fatal message
func Tracef(format string, v ...interface{}) {
	if LogLevel(Logger().zlog.GetLevel()) <= DebugLevel {
		Logger().zlog.Trace().Stack().Msgf(format, v...)
	}
}

// Panic logs a fatal message
func Panic(msg ...interface{}) {
	Logger().zlog.Panic().Stack().Msg(parseInterface(msg))
}

// Panicf logs a fatal message
func Panicf(format string, v ...interface{}) {
	Logger().zlog.Panic().Stack().Msgf(format, v...)
}

// Level returns the current log level
func Level() LogLevel {
	return LogLevel(Logger().zlog.GetLevel())
}

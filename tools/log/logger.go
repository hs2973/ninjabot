// Package log provides a wrapper around the logrus logging library with convenience
// functions for different log levels. It offers type aliases for common logrus types
// and simplified logging methods to reduce boilerplate code throughout the application.
package log

import "github.com/sirupsen/logrus"

// Log level constants that map to logrus levels for convenient access
var (
	WarnLevel  = logrus.WarnLevel
	InfoLevel  = logrus.InfoLevel
	DebugLevel = logrus.DebugLevel
	ErrorLevel = logrus.ErrorLevel
	FatalLevel = logrus.FatalLevel
	PanicLevel = logrus.PanicLevel
)

// Type aliases for logrus types to provide a cleaner API
type (
	TextFormatter = logrus.TextFormatter
	Level         = logrus.Level
)

// CheckErr logs an error message at the specified level if the error is not nil.
// This is a convenience function for conditional error logging throughout the application.
func CheckErr(level logrus.Level, err error) {
	if err != nil {
		Log(level, err)
	}
}

// Log outputs a message at the specified log level using the appropriate logrus function.
// This provides a unified interface for logging at different levels with automatic
// fallback to debug level for unrecognized levels.
func Log(level logrus.Level, messages ...interface{}) {
	switch level {
	case logrus.InfoLevel:
		logrus.Info(messages...)
	case logrus.WarnLevel:
		logrus.Warn(messages...)
	case logrus.ErrorLevel:
		logrus.Error(messages...)
	case logrus.FatalLevel:
		logrus.Fatal(messages...)
	case logrus.PanicLevel:
		logrus.Panic(messages...)
	case logrus.DebugLevel:
		fallthrough
	default:
		logrus.Debug(messages...)
	}
}

// SetFormatter configures the output format for log messages.
// This allows customization of how log messages are displayed or formatted.
func SetFormatter(formatter logrus.Formatter) {
	logrus.SetFormatter(formatter)
}

// SetLevel configures the minimum log level for output.
// Messages below this level will be suppressed.
func SetLevel(level logrus.Level) {
	logrus.SetLevel(level)
}

// WithField creates a log entry with a single field for structured logging.
// This is useful for adding context to log messages without string formatting.
func WithField(key string, value interface{}) *logrus.Entry {
	return logrus.WithField(key, value)
}

// WithFields creates a log entry with multiple fields for structured logging.
// This allows adding multiple context fields to log messages efficiently.
func WithFields(fields logrus.Fields) *logrus.Entry {
	return logrus.WithFields(fields)
}

// Info logs a message at Info level for general application information.
func Info(messages ...interface{}) {
	logrus.Info(messages...)
}

// Infof logs a formatted message at Info level for general application information.
func Infof(format string, messages ...interface{}) {
	logrus.Infof(format, messages...)
}

// Warn logs a message at Warning level for potentially problematic situations.
func Warn(messages ...interface{}) {
	logrus.Warn(messages...)
}

// Warnf logs a formatted message at Warning level for potentially problematic situations.
func Warnf(format string, messages ...interface{}) {
	logrus.Warnf(format, messages...)
}

// Error logs a message at Error level for error conditions that should be investigated.
func Error(messages ...interface{}) {
	logrus.Error(messages...)
}

// Errorf logs a formatted message at Error level for error conditions that should be investigated.
func Errorf(format string, messages ...interface{}) {
	logrus.Errorf(format, messages...)
}

// Fatal logs a message at Fatal level and terminates the application.
// Use this for unrecoverable errors that require immediate program termination.
func Fatal(messages ...interface{}) {
	logrus.Fatal(messages...)
}

// Debug logs a message at Debug level for detailed diagnostic information.
// These messages are typically only shown during development or troubleshooting.
func Debug(messages ...interface{}) {
	logrus.Debug(messages...)
}

// Debugf logs a formatted message at Debug level for detailed diagnostic information.
// These messages are typically only shown during development or troubleshooting.
func Debugf(format string, messages ...interface{}) {
	logrus.Debugf(format, messages...)
}

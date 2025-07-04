package ux

// Prompter abstracts user interaction for testability
type Prompter interface {
	Select(message string, options []string) (string, error)
	Confirm(message string) (bool, error)
	Input(message string) (string, error)
}

// MultiSelectPrompter extends Prompter with multi-selection capability
type MultiSelectPrompter interface {
	Prompter
	MultiSelect(message string, options []string) ([]string, error)
}

// Logger abstracts logging for structured output and testing
type Logger interface {
	Info(msg string, fields ...LogField)
	Warn(msg string, fields ...LogField)
	Error(msg string, fields ...LogField)
	Debug(msg string, fields ...LogField)
}

// LogField represents a structured log field
type LogField struct {
	Key   string
	Value interface{}
}

func Field(key string, value interface{}) LogField {
	return LogField{Key: key, Value: value}
}

package broker

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

type Logger interface {
	Info(msg string, fields map[string]any)
	Error(msg string, fields map[string]any)
}

type JSONLogger struct {
	logger *log.Logger
}

func NewJSONLogger() *JSONLogger {
	return &JSONLogger{logger: log.New(os.Stdout, "", 0)}
}

func (l *JSONLogger) Info(msg string, fields map[string]any) {
	l.log("info", msg, fields)
}

func (l *JSONLogger) Error(msg string, fields map[string]any) {
	l.log("error", msg, fields)
}

func (l *JSONLogger) log(level, msg string, fields map[string]any) {
	payload := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"level": level,
		"msg":   msg,
	}
	for k, v := range fields {
		payload[k] = v
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		l.logger.Printf("{\"ts\":%q,\"level\":%q,\"msg\":%q}", payload["ts"], level, "log_marshal_failed")
		return
	}
	l.logger.Print(string(blob))
}

type TextLogger struct {
	logger *log.Logger
}

func NewTextLogger() *TextLogger {
	return &TextLogger{logger: log.New(os.Stdout, "", log.LstdFlags)}
}

func (l *TextLogger) Info(msg string, fields map[string]any) {
	l.logger.Printf("INFO %s %v", msg, fields)
}

func (l *TextLogger) Error(msg string, fields map[string]any) {
	l.logger.Printf("ERROR %s %v", msg, fields)
}

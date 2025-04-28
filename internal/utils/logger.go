package utils

import (
	"io"
	"log"
	"time"

	"go.mau.fi/whatsmeow/types"
)

var (
	InfoLogger    *log.Logger
	ErrorLogger   *log.Logger
	DebugLogger   *log.Logger
	WarningLogger *log.Logger
)

func init() {
	// Use discard writer to effectively disable logging
	InfoLogger = log.New(io.Discard, "", 0)
	ErrorLogger = log.New(io.Discard, "", 0)
	DebugLogger = log.New(io.Discard, "", 0)
	WarningLogger = log.New(io.Discard, "", 0)
}

func LogDebug(format string, v ...interface{}) {
	// No-op function
}

func LogInfo(format string, v ...interface{}) {
	// No-op function
}

func LogError(format string, v ...interface{}) {
	// No-op function
}

func LogWarning(format string, v ...interface{}) {
	// No-op function
}

func TimeTrack(start time.Time, name string) {
	// No-op function
}

func GetCurrentTimestamp() int64 {
	return time.Now().Unix()
}

func ParseJID(recipient string) (types.JID, error) {
	if len(recipient) == 11 || len(recipient) == 10 {
		recipient = "55" + recipient
	}
	return types.ParseJID(recipient + "@s.whatsapp.net")
}

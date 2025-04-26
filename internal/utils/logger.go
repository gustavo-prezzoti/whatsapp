package utils

import (
	"fmt"
	"log"
	"os"
	"runtime"
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
	InfoLogger = log.New(os.Stdout, "[INFO] ", log.Ldate|log.Ltime)
	ErrorLogger = log.New(os.Stderr, "[ERROR] ", log.Ldate|log.Ltime)
	DebugLogger = log.New(os.Stdout, "[DEBUG] ", log.Ldate|log.Ltime)
	WarningLogger = log.New(os.Stdout, "[WARNING] ", log.Ldate|log.Ltime)
}

func LogDebug(format string, v ...interface{}) {
	_, file, line, _ := runtime.Caller(1)
	msg := fmt.Sprintf(format, v...)
	DebugLogger.Printf("[%s:%d] %s", file, line, msg)
}

func LogInfo(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	InfoLogger.Printf("%s", msg)
}

func LogError(format string, v ...interface{}) {
	_, file, line, _ := runtime.Caller(1)
	msg := fmt.Sprintf(format, v...)
	ErrorLogger.Printf("[%s:%d] %s", file, line, msg)
}

func LogWarning(format string, v ...interface{}) {
	_, file, line, _ := runtime.Caller(1)
	msg := fmt.Sprintf(format, v...)
	WarningLogger.Printf("[%s:%d] %s", file, line, msg)
}

func TimeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	LogDebug("%s levou %s", name, elapsed)
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

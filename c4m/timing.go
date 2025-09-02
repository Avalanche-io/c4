package c4m

import (
	"fmt"
	"os"
	"time"
)

// StartTime records the application start time for elapsed time calculations
var StartTime time.Time

func init() {
	StartTime = time.Now()
}

// ElapsedTime returns the elapsed time since start as a formatted string
func ElapsedTime() string {
	elapsed := time.Since(StartTime)
	hours := int(elapsed.Hours())
	minutes := int(elapsed.Minutes()) % 60
	seconds := int(elapsed.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

// TimedPrintf prints a message with elapsed time prefix to stderr
func TimedPrintf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[%s] %s", ElapsedTime(), msg)
}

// TimedPrintln prints a message with elapsed time prefix to stderr
func TimedPrintln(msg string) {
	fmt.Fprintf(os.Stderr, "[%s] %s\n", ElapsedTime(), msg)
}
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var logFile = func() *os.File {
	homeDir, err := os.UserHomeDir()
	ce(err)
	f, err := os.OpenFile(
		filepath.Join(homeDir, ".ovi-logs"),
		os.O_APPEND|os.O_CREATE|os.O_RDWR,
		0644,
	)
	ce(err)
	return f
}()

var logFile0 = func() *os.File {
	homeDir, err := os.UserHomeDir()
	ce(err)
	f, err := os.OpenFile(
		filepath.Join(homeDir, ".ovi-logs.0"),
		os.O_CREATE|os.O_RDWR|os.O_TRUNC,
		0644,
	)
	ce(err)
	return f
}()

var outputFileLock sync.Mutex

func log(format string, args ...interface{}) {
	outputFileLock.Lock()
	defer outputFileLock.Unlock()
	fmt.Fprintf(logFile, "%s ", time.Now().Format("15:04:05.999"))
	fmt.Fprintf(logFile, format, args...)
	logFile.Sync()
	fmt.Fprintf(logFile0, "%s ", time.Now().Format("15:04:05.999"))
	fmt.Fprintf(logFile0, format, args...)
	logFile0.Sync()
}

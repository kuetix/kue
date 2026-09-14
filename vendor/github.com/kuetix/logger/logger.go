package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

// Log levels
// 0000: Quite
// 0001: Info
// 0010: Debug
// 0100: Warn
// 1000: Error
// 1111: All
//
//goland:noinspection GoUnusedConst
const (
	LogLevelQuite   = 0x00                                                                         // 0000
	LogLevelInfo    = 1 << iota                                                                    // 0001
	LogLevelDebug                                                                                  // 0010
	LogLevelWarn                                                                                   // 0100
	LogLevelError                                                                                  // 0100
	LogLevelFatal                                                                                  // 1000
	LogLevelPanic                                                                                  // 1000
	LogLevelAll     = LogLevelInfo | LogLevelDebug | LogLevelError | LogLevelFatal | LogLevelPanic // 1111
	LogLevelNone    = LogLevelQuite                                                                // 0000
	LogLevelDefault = LogLevelQuite | LogLevelError                                                // Default log level, nothing enabled except errors

)

//goland:noinspection GoUnusedConst
var (
	FormatV   = "%05d: %s/%s:%d %v"
	FormatVnl = "%05d: %s/%s:%d %v\n"
	FormatF   = "%05d: %s/%s:%d "

	JsonFormatEnabled      = false
	LogLevel          byte = LogLevelDefault // nothing enabled
	StdErr                 = os.Stderr
	StdOut                 = os.Stdout
	loggerLineCount        = 0
	infoLogger             = log.New(StdOut, "[INFO] ", log.LstdFlags|log.Ldate|log.Ltime)
	debugLogger            = log.New(StdOut, "[DEBUG] ", log.LstdFlags|log.Ldate|log.Ltime)
	warnLogger             = log.New(StdOut, "[WARN] ", log.LstdFlags|log.Ldate|log.Ltime)
	errorLogger            = log.New(StdErr, "[ERROR] ", log.LstdFlags|log.Ldate|log.Ltime)
	fatalLogger            = log.New(StdErr, "[FATAL] ", log.LstdFlags|log.Ldate|log.Ltime)
	panicLogger            = log.New(StdErr, "[PANIC] ", log.LstdFlags|log.Ldate|log.Ltime)
)

//goland:noinspection GoUnusedGlobalVariable
var loggerFile *os.File

// SetJsonFormat enables or disables JSON format for log output
//
//goland:noinspection GoUnusedExportedFunction
func SetJsonFormat(enabled bool) {
	if enabled {
		JsonFormatEnabled = true
		FormatV = "\", \"No\": \"%05d\", \"file\": \"%s/%s:%d\", \"message\": \"%v\"}"
		FormatVnl = "\", \"No\": \"%05d\", \"file\": \"%s/%s:%d\", \"message\": \"%v\"}\n"
		FormatF = "\", \"No\": \"%05d\", \"file\": \"%s/%s:%d\", \"message\": \""

		infoLogger.SetPrefix("{\"level\":\"INFO\",\"time\":\"")
		debugLogger.SetPrefix("{\"level\":\"DEBUG\",\"time\":\"")
		warnLogger.SetPrefix("{\"level\":\"WARN\",\"time\":\"")
		errorLogger.SetPrefix("{\"level\":\"ERROR\",\"time\":\"")
		fatalLogger.SetPrefix("{\"level\":\"FATAL\",\"time\":\"")
		panicLogger.SetPrefix("{\"level\":\"PANIC\",\"time\":\"")
	} else {
		JsonFormatEnabled = false
		FormatV = "%05d: %s/%s:%d %v"
		FormatVnl = "%05d: %s/%s:%d %v\n"
		FormatF = "%05d: %s/%s:%d "

		infoLogger.SetPrefix("[INFO] ")
		debugLogger.SetPrefix("[DEBUG] ")
		warnLogger.SetPrefix("[WARN] ")
		errorLogger.SetPrefix("[ERROR] ")
		fatalLogger.SetPrefix("[FATAL] ")
		panicLogger.SetPrefix("[PANIC] ")
	}
}

// EnableQuiet turns on debug logs
func EnableQuiet() {
	LogLevel = 0x00
}

// DisableQuiet turns off debug logs
//
//goland:noinspection GoUnusedExportedFunction
func DisableQuiet() {
	LogLevel = LogLevelInfo
}

// EnableInfo turns on debug logs
func EnableInfo() {
	LogLevel = LogLevel | LogLevelInfo
}

// DisableInfo turns off debug logs
//
//goland:noinspection GoUnusedExportedFunction
func DisableInfo() {
	LogLevel = LogLevel &^ LogLevelInfo
}

// EnableDebug turns on debug logs
func EnableDebug() {
	LogLevel = LogLevel | LogLevelDebug
}

// DisableDebug turns off debug logs
//
//goland:noinspection GoUnusedExportedFunction
func DisableDebug() {
	LogLevel = LogLevel &^ LogLevelDebug
}

// EnableWarn turns on debug logs
func EnableWarn() {
	LogLevel = LogLevel | LogLevelWarn
}

// DisableWarn turns off debug logs
func DisableWarn() {
	LogLevel = LogLevel &^ LogLevelWarn
}

// EnableError turns on debug logs
func EnableError() {
	LogLevel = LogLevel | LogLevelError
}

// DisableError turns off debug logs
func DisableError() {
	LogLevel = LogLevel &^ LogLevelError
}

// EnableFatal turns on debug logs
func EnableFatal() {
	LogLevel = LogLevel | LogLevelFatal
}

// DisableFatal turns off debug logs
func DisableFatal() {
	LogLevel = LogLevel &^ LogLevelFatal
}

// EnablePanic turns on debug logs
func EnablePanic() {
	LogLevel = LogLevel | LogLevelPanic
}

// DisablePanic turns off debug logs
func DisablePanic() {
	LogLevel = LogLevel &^ LogLevelPanic
}

// SetOutputInfo sets the output destination for info logs
//
//goland:noinspection GoUnusedExportedFunction
func SetOutputInfo(output *os.File) {
	infoLogger.SetOutput(output)
}

// SetOutputDebug sets the output destination for debug logs
//
//goland:noinspection GoUnusedExportedFunction
func SetOutputDebug(output *os.File) {
	debugLogger.SetOutput(output)
}

// SetOutputWarn sets the output destination for warn logs
//
//goland:noinspection GoUnusedExportedFunction
func SetOutputWarn(output *os.File) {
	warnLogger.SetOutput(output)
}

// SetOutputError sets the output destination for error logs
//
//goland:noinspection GoUnusedExportedFunction
func SetOutputError(output *os.File) {
	errorLogger.SetOutput(output)
}

// SetOutputFatal sets the output destination for fatal logs
//
//goland:noinspection GoUnusedExportedFunction
func SetOutputFatal(output *os.File) {
	fatalLogger.SetOutput(output)
}

// SetOutputPanic sets the output destination for panic logs
//
//goland:noinspection GoUnusedExportedFunction
func SetOutputPanic(output *os.File) {
	panicLogger.SetOutput(output)
}

// SetOutput sets the output destination for all log levels
//
//goland:noinspection GoUnusedExportedFunction
func SetOutput(output *os.File) {
	infoLogger.SetOutput(output)
	debugLogger.SetOutput(output)
	warnLogger.SetOutput(output)
	errorLogger.SetOutput(output)
	fatalLogger.SetOutput(output)
	panicLogger.SetOutput(output)
}

// SetOutputPath sets the output destination for all log levels
//
//goland:noinspection GoUnusedExportedFunction
func SetOutputPath(outputPath string) {
	var err error
	basedir := filepath.Dir(outputPath)
	err = os.MkdirAll(basedir, os.ModePerm)
	if err != nil {
		errorLogger.Printf("Failed to create log directory %s: %v", basedir, err)
		return
	}
	loggerFile, err = os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		errorLogger.Printf("Failed to open log file %s: %v", outputPath, err)
		return
	}
	infoLogger.SetOutput(loggerFile)
	debugLogger.SetOutput(loggerFile)
	warnLogger.SetOutput(loggerFile)
	errorLogger.SetOutput(loggerFile)
	fatalLogger.SetOutput(loggerFile)
	panicLogger.SetOutput(loggerFile)
}

// CloseLogFile closes the log file
func CloseLogFile() {
	if loggerFile != nil {
		_ = loggerFile.Close()
	}
}

// FileErrorf returns the file, line and working directory of the caller
func FileErrorf(skip int) (string, int, string) {
	var err error
	_, file, line, ok := runtime.Caller(skip + 1)
	var wd string
	if ok {
		wd, err = os.Getwd()
		if err != nil {
			wd = ""
		}
		var split string
		split, file = filepath.Split(file)
		wd, _ = RemoveCommonPrefix(split, wd)
		file = filepath.Base(file)
	} else {
		file = "unknown_file"
		line = 0
	}
	return file, line, wd
}

// Debug works like log.Println but only when enabled
func Debug(v ...interface{}) {
	file, line, wd := FileErrorf(1)
	if LogLevel&LogLevelDebug != 0 {
		loggerLineCount++
		v = append([]interface{}{loggerLineCount, wd, file, line}, v...)
		debugLogger.Printf(FormatV, v...)
	}
}

// Debugf works like log.Printf but only when enabled
//
//goland:noinspection SpellCheckingInspection
func Debugf(format string, v ...interface{}) {
	file, line, wd := FileErrorf(1)
	if LogLevel&LogLevelDebug != 0 {
		loggerLineCount++
		v = append([]interface{}{loggerLineCount, wd, file, line}, v...)
		if JsonFormatEnabled {
			v = append(v, "\"}")
		}
		debugLogger.Printf(FormatF+format, v...)
	}
}

// Info works like log.Println
func Info(v ...interface{}) {
	loggerLineCount++
	file, line, wd := FileErrorf(1)
	if LogLevel&LogLevelInfo != 0 {
		v = append([]interface{}{loggerLineCount, wd, file, line}, v...)
		infoLogger.Printf(FormatV, v...)
	}
}

// Infof works like log.Printf
//
//goland:noinspection GoUnusedExportedFunction
func Infof(format string, v ...interface{}) {
	file, line, wd := FileErrorf(1)
	if LogLevel&LogLevelInfo != 0 {
		loggerLineCount++
		v = append([]interface{}{loggerLineCount, wd, file, line}, v...)
		if JsonFormatEnabled {
			v = append(v, "\"}")
		}
		infoLogger.Printf(FormatF+format, v...)
	}
}

// Warn works like log.Println
//
//goland:noinspection GoUnusedExportedFunction
func Warn(v ...interface{}) {
	loggerLineCount++
	file, line, wd := FileErrorf(1)
	if LogLevel&LogLevelInfo != 0 {
		v = append([]interface{}{loggerLineCount, wd, file, line}, v...)
		warnLogger.Printf(FormatV, v...)
	}
}

// Warnf works like log.Printf
func Warnf(format string, v ...interface{}) {
	file, line, wd := FileErrorf(1)
	if LogLevel&LogLevelInfo != 0 {
		loggerLineCount++
		v = append([]interface{}{loggerLineCount, wd, file, line}, v...)
		if JsonFormatEnabled {
			v = append(v, "\"}")
		}
		warnLogger.Printf(FormatF+format, v...)
	}
}

// Error works like log.Println
func Error(v ...interface{}) {
	file, line, wd := FileErrorf(1)
	if LogLevel&LogLevelError != 0 {
		loggerLineCount++
		v = append([]interface{}{loggerLineCount, wd, file, line}, v...)
		errorLogger.Printf(FormatVnl, v...)
	}
}

// SErrorf works like log.Printf and returns the formatted string
func SErrorf(format string, v ...interface{}) error {
	file, line, wd := FileErrorf(1)
	i := append([]interface{}{loggerLineCount, wd, file, line}, v...)
	return fmt.Errorf(FormatF+format, i...)
}

// Errorf works like log.Printf
func Errorf(format string, v ...interface{}) {
	file, line, wd := FileErrorf(1)
	if LogLevel&LogLevelError != 0 {
		loggerLineCount++
		v = append([]interface{}{loggerLineCount, wd, file, line}, v...)
		if JsonFormatEnabled {
			v = append(v, "\"\"}")
		}
		errorLogger.Printf(FormatF+format, v...)
	}
}

// Fatal works like log.Fatal
//
//goland:noinspection GoUnusedExportedFunction
func Fatal(v ...interface{}) {
	loggerLineCount++
	file, line, wd := FileErrorf(1)
	v = append([]interface{}{loggerLineCount, wd, file, line}, v...)
	fatalLogger.Fatalf(FormatV, v...)
}

// Fatalf works like log.Fatalf
//
//goland:noinspection GoUnusedExportedFunction
func Fatalf(format string, v ...interface{}) {
	loggerLineCount++
	file, line, wd := FileErrorf(1)
	v = append([]interface{}{loggerLineCount, wd, file, line}, v...)
	if JsonFormatEnabled {
		v = append(v, "\"}")
	}
	fatalLogger.Fatalf(FormatF+format, v...)
}

// Panic works like log.Panic
//
//goland:noinspection GoUnusedExportedFunction
func Panic(v ...interface{}) {
	loggerLineCount++
	file, line, wd := FileErrorf(1)
	v = append([]interface{}{loggerLineCount, wd, file, line}, v...)
	panicLogger.Panicf(FormatV, v...)
}

// Panicf works like a log.Panicf
//
//goland:noinspection GoUnusedExportedFunction
func Panicf(format string, v ...interface{}) {
	loggerLineCount++
	file, line, wd := FileErrorf(1)
	v = append([]interface{}{loggerLineCount, wd, file, line}, v...)
	if JsonFormatEnabled {
		v = append(v, "\"}")
	}
	panicLogger.Panicf(FormatF+format, v...)
}

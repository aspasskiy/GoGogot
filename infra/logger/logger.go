package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var logFile *os.File

func Init(dataDir, logLevel string) error {
	dir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("logger: mkdir: %w", err)
	}

	name := fmt.Sprintf("gogogot-%s.log", time.Now().Format("2006-01-02"))
	path := filepath.Join(dir, name)

	var err error
	logFile, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("logger: open: %w", err)
	}

	console := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "15:04:05",
		FormatLevel: func(i interface{}) string {
			lvl := strings.ToUpper(fmt.Sprint(i))
			switch lvl {
			case "DEBUG":
				return "\033[1;34mDBG\033[0m"
			case "INFO":
				return "\033[1;36mINF\033[0m"
			case "WARN":
				return "\033[1;33mWRN\033[0m"
			case "ERROR":
				return "\033[1;31mERR\033[0m"
			case "FATAL":
				return "\033[1;35mFTL\033[0m"
			default:
				return lvl
			}
		},
		FormatMessage: func(i interface{}) string {
			return fmt.Sprintf("\033[1mgogogot\033[0m %s", i)
		},
	}

	fileLevel := parseLevel(logLevel)

	fileWriter := &levelWriter{w: logFile, level: fileLevel}
	multi := io.MultiWriter(console, fileWriter)

	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	zerolog.TimeFieldFormat = time.RFC3339

	log.Logger = zerolog.New(multi).With().Timestamp().Logger()

	log.Info().Str("path", path).Str("file_level", fileLevel.String()).Msg("logger initialized")
	return nil
}

func Close() {
	if logFile != nil {
		_ = logFile.Sync()
		_ = logFile.Close()
	}
}

type levelWriter struct {
	w     io.Writer
	level zerolog.Level
}

func (lw *levelWriter) Write(p []byte) (n int, err error) {
	return lw.w.Write(p)
}

func (lw *levelWriter) WriteLevel(level zerolog.Level, p []byte) (n int, err error) {
	if level < lw.level {
		return len(p), nil
	}
	return lw.w.Write(p)
}

func parseLevel(s string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return zerolog.DebugLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

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

// Init sets up the global logger. Logs go to stderr, or — when file is set,
// e.g. while the terminal channel owns the screen — are appended to file.
func Init(logLevel, file string) error {
	var out io.Writer = os.Stderr
	noColor := false
	if file != "" {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		out, noColor = f, true
	}

	console := zerolog.ConsoleWriter{
		Out:        out,
		NoColor:    noColor,
		TimeFormat: "15:04:05",
		FormatMessage: func(i interface{}) string {
			if noColor {
				return fmt.Sprintf("gogogot %s", i)
			}
			return fmt.Sprintf("\033[1mgogogot\033[0m %s", i)
		},
	}

	level := parseLevel(logLevel)
	zerolog.SetGlobalLevel(level)
	zerolog.TimeFieldFormat = time.RFC3339

	log.Logger = zerolog.New(console).With().Timestamp().Logger()

	log.Info().Str("level", level.String()).Msg("logger initialized")
	return nil
}

func parseLevel(s string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace", "debug":
		return zerolog.TraceLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

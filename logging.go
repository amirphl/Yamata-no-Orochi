package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/amirphl/Yamata-no-Orochi/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

// setupLogging configures the global logger to write to a rotating file (and optionally stdout)
// so logs survive container restarts when the path is backed by a volume.
func setupLogging(cfg config.LoggingConfig) (io.Closer, error) {
	output := strings.ToLower(strings.TrimSpace(cfg.Output))
	if output == "" {
		output = "file"
	}

	var writers []io.Writer
	var closer io.Closer

	if output == "stdout" || output == "both" {
		writers = append(writers, os.Stdout)
	}

	if output == "file" || output == "both" {
		path := cfg.FilePath
		if path == "" {
			path = "/var/log/yamata/app.log"
		}

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}

		lj := &lumberjack.Logger{
			Filename:   path,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}

		writers = append(writers, lj)
		closer = lj
	}

	// Fallback to stdout when configuration leaves us with no target
	if len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}

	log.SetOutput(io.MultiWriter(writers...))

	flags := log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC
	if cfg.EnableCaller {
		flags |= log.Lshortfile
	}
	log.SetFlags(flags)

	return closer, nil
}

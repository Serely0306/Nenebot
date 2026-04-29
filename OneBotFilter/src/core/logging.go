package core

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

func SetupRuntimeLogging(logFilePath string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(logFilePath), 0o755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	prevWriter := log.Writer()
	log.SetOutput(io.MultiWriter(os.Stdout, file))

	return func() error {
		log.SetOutput(prevWriter)
		return file.Close()
	}, nil
}

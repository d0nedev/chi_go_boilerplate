package logging

import (
	"log/slog"
	"os"
)

type Config struct {
	ServiceName string
	Environment string
	Level       slog.Level
}

func New(cfg Config) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: cfg.Environment == "development",
	})

	return slog.New(handler).With(
		slog.String("service", cfg.ServiceName),
		slog.String("env", cfg.Environment),
	)
}

package logger

import (
	"github.com/gofurry/sagaflow/internal/config"
	"go.uber.org/zap"
)

func New(app config.AppConfig) (*zap.Logger, error) {
	var cfg zap.Config
	if app.Env == "prod" || app.Env == "production" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.DisableStacktrace = true
	}
	cfg.InitialFields = map[string]any{
		"app":     app.Name,
		"env":     app.Env,
		"version": app.Version,
	}
	return cfg.Build()
}

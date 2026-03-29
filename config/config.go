package config

import (
	"log/slog"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Log     LogConfig     `mapstructure:"log"`
	Copilot CopilotConfig `mapstructure:"copilot"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

type CopilotConfig struct {
	Token    string `mapstructure:"token"`
	Model    string `mapstructure:"model"`
	Endpoint string `mapstructure:"endpoint"`
}

// SlogLevel converts the configured log level string to slog.Level.
func (c *LogConfig) SlogLevel() slog.Level {
	switch strings.ToLower(c.Level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Load reads configuration from file and environment variables.
// It looks for config.yaml in the current directory.
func Load() (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("log.level", "info")
	v.SetDefault("copilot.model", "gpt-4o")
	v.SetDefault("copilot.endpoint", "https://api.githubcopilot.com/chat/completions")

	// Config file
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")

	// Environment variables: ZZY_LOG_LEVEL, ZZY_COPILOT_TOKEN, etc.
	v.SetEnvPrefix("ZZY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

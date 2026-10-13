package config

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// DefaultTablePrefix is used when no table prefix is configured.
const DefaultTablePrefix = "seonaut_"

// ErrMissingAPIKey is returned when no API key is configured.
var ErrMissingAPIKey = errors.New("api key is required (set [api] key in the config file or the SEONAUT_API_KEY environment variable)")

// CrawlerConfig stores the configuration for the crawler.
type CrawlerConfig struct {
	Agent string `mapstructure:"agent"`
}

// HTTPServerConfig stores the configuration for the HTTP server.
type HTTPServerConfig struct {
	Server string `mapstructure:"host"`
	Port   int    `mapstructure:"port"`
	URL    string `mapstructure:"url"`
}

// DBConfig stores the configuration for the database store.
type DBConfig struct {
	Server string `mapstructure:"server"`
	Port   int    `mapstructure:"port"`
	User   string `mapstructure:"user"`
	Pass   string `mapstructure:"password"`
	Name   string `mapstructure:"database"`

	// TablePrefix is prepended to every table name, including the
	// migrations table. Defaults to "seonaut_".
	TablePrefix string `mapstructure:"table_prefix"`
}

// APIConfig stores the settings for the JSON API.
type APIConfig struct {
	Key string `mapstructure:"key"`
}

// Config stores the configuration for the application.
type Config struct {
	Crawler    *CrawlerConfig    `mapstructure:"crawler"`
	HTTPServer *HTTPServerConfig `mapstructure:"server"`
	DB         *DBConfig         `mapstructure:"database"`
	API        *APIConfig        `mapstructure:"api"`
}

// NewConfig loads the configuration from the specified file and path.
// It returns ErrMissingAPIKey if no API key has been configured.
func NewConfig(configFile string) (*Config, error) {
	viper.AddConfigPath(filepath.Dir(configFile))
	viper.SetConfigName(filepath.Base(configFile))
	viper.SetConfigType("toml")

	// Enable environment variable support
	viper.AutomaticEnv()
	viper.SetEnvPrefix("SEONAUT") // Optional: prefix for env vars
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Register the key so that SEONAUT_API_KEY is picked up even when the
	// [api] section is missing from the config file.
	viper.SetDefault("api.key", "")
	viper.SetDefault("database.table_prefix", DefaultTablePrefix)

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config

	err := viper.Unmarshal(&config)
	if err != nil {
		return nil, err
	}

	if config.API == nil || strings.TrimSpace(config.API.Key) == "" {
		return nil, ErrMissingAPIKey
	}

	return &config, nil
}

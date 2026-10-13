package config_test

import (
	"errors"
	"testing"

	"github.com/stjudewashere/seonaut/internal/config"
)

func TestLoadConfig(t *testing.T) {
	config, err := config.NewConfig("./testdata/config")
	if err != nil {
		t.Fatalf("Error loading config file: %v", err)
	}

	pm := []struct {
		input int
		want  int
	}{
		{config.HTTPServer.Port, 9000},
		{config.DB.Port, 3306},
	}

	for _, pv := range pm {
		if pv.input != pv.want {
			t.Errorf("%d != %d\n", pv.input, pv.want)
		}
	}

	m := []struct {
		input string
		want  string
	}{
		{config.HTTPServer.Server, "example.com"},
		{config.DB.Server, "dbexample.com"},
		{config.DB.User, "root"},
		{config.DB.Pass, "root"},
		{config.DB.Name, "test"},
		{config.Crawler.Agent, "testing"},
		{config.API.Key, "test-key"},
		{config.DB.TablePrefix, "seonaut_"},
	}

	for _, v := range m {
		if v.input != v.want {
			t.Errorf("%s != %s\n", v.input, v.want)
		}
	}
}

func TestAPIKeyFromEnv(t *testing.T) {
	t.Setenv("SEONAUT_API_KEY", "env-key")

	c, err := config.NewConfig("./testdata/config_noapi")
	if err != nil {
		t.Fatalf("Error loading config file: %v", err)
	}

	if c.API.Key != "env-key" {
		t.Errorf("API key %q != env-key", c.API.Key)
	}
}

func TestTablePrefixFromEnv(t *testing.T) {
	t.Setenv("SEONAUT_API_KEY", "env-key")
	t.Setenv("SEONAUT_DATABASE_TABLE_PREFIX", "custom_")

	c, err := config.NewConfig("./testdata/config_noapi")
	if err != nil {
		t.Fatalf("Error loading config file: %v", err)
	}

	if c.DB.TablePrefix != "custom_" {
		t.Errorf("table prefix %q != custom_", c.DB.TablePrefix)
	}
}

func TestMissingAPIKey(t *testing.T) {
	t.Setenv("SEONAUT_API_KEY", "")

	_, err := config.NewConfig("./testdata/config_noapi")
	if !errors.Is(err, config.ErrMissingAPIKey) {
		t.Errorf("expected ErrMissingAPIKey, got %v", err)
	}
}

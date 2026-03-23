package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_EnvInterpolation(t *testing.T) {
	t.Setenv("TEST_CORALOGIX_KEY", "test-api-key")
	t.Setenv("TEST_SLACK_URL", "https://hooks.slack.com/test")

	yaml := `
coralogix:
  api_key: ${TEST_CORALOGIX_KEY}
  endpoint: https://api.coralogix.us/api/v1/dataprime/query
notifications:
  slack:
    webhook_url: ${TEST_SLACK_URL}
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Coralogix.APIKey != "test-api-key" {
		t.Errorf("APIKey = %q, want %q", cfg.Coralogix.APIKey, "test-api-key")
	}
	if cfg.Notifications.Slack.WebhookURL != "https://hooks.slack.com/test" {
		t.Errorf("WebhookURL = %q, want %q", cfg.Notifications.Slack.WebhookURL, "https://hooks.slack.com/test")
	}
}

func TestLoad_EnvInterpolation_Unset(t *testing.T) {
	os.Unsetenv("TEST_MISSING_VAR")

	yaml := `
coralogix:
  api_key: ${TEST_MISSING_VAR}
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Coralogix.APIKey != "" {
		t.Errorf("APIKey = %q, want empty string for unset var", cfg.Coralogix.APIKey)
	}
}

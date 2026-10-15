package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenyme/grok2api/backend/internal/infra/config"
)

func clearInitializationEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"GROK2API_ADMIN_USERNAME", "GROK2API_ADMIN_PASSWORD", "ZEABUR", "PASSWORD", config.DatabaseURLEnv} {
		t.Setenv(name, "")
	}
}

func TestInitializeConfigGeneratesAndPreservesSecrets(t *testing.T) {
	clearInitializationEnvironment(t)
	path := filepath.Join(t.TempDir(), "data", "config.yaml")
	var output bytes.Buffer
	if err := initializeConfigFile(path, &output); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BootstrapAdmin.Username != "admin" || len(cfg.BootstrapAdmin.Password) < 32 {
		t.Fatal("initial administrator was not generated")
	}
	if !strings.Contains(output.String(), cfg.BootstrapAdmin.Password) {
		t.Fatal("generated administrator password was not reported")
	}
	if strings.Contains(output.String(), cfg.Secrets.JWTSecret) || strings.Contains(output.String(), cfg.Secrets.CredentialEncryptionKey) {
		t.Fatal("application secrets were printed")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("generated config must have mode 0600")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GROK2API_ADMIN_PASSWORD", "another-password")
	output.Reset()
	if err := initializeConfigFile(path, &output); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing configuration was changed")
	}
	if strings.Contains(output.String(), "Initial admin password") {
		t.Fatal("existing credentials were printed again")
	}
}

func TestInitializeConfigAdministratorEnvironment(t *testing.T) {
	for _, test := range []struct {
		name     string
		zeabur   string
		explicit string
		want     string
	}{
		{name: "Zeabur password", zeabur: "1", want: "zeabur-test-password"},
		{name: "explicit takes precedence", zeabur: "1", explicit: "quoted 'password': \"value\"\nsecond line", want: "quoted 'password': \"value\"\nsecond line"},
		{name: "generic password ignored outside Zeabur"},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearInitializationEnvironment(t)
			t.Setenv("ZEABUR", test.zeabur)
			t.Setenv("PASSWORD", "zeabur-test-password")
			t.Setenv("GROK2API_ADMIN_USERNAME", "operator")
			t.Setenv("GROK2API_ADMIN_PASSWORD", test.explicit)
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := initializeConfigFile(path, io.Discard); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.BootstrapAdmin.Username != "operator" {
				t.Fatal("administrator username override was ignored")
			}
			if test.want != "" && cfg.BootstrapAdmin.Password != test.want {
				t.Fatal("administrator password was not preserved through YAML")
			}
			if test.want == "" && cfg.BootstrapAdmin.Password == "zeabur-test-password" {
				t.Fatal("generic PASSWORD was used outside Zeabur")
			}
		})
	}
}

func TestInitializeConfigRejectsInvalidPasswordsWithoutPublishing(t *testing.T) {
	for _, password := range []string{"short", strings.Repeat("x", 73), "replace-with-a-strong-password"} {
		clearInitializationEnvironment(t)
		t.Setenv("GROK2API_ADMIN_PASSWORD", password)
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		if err := initializeConfigFile(path, io.Discard); err == nil {
			t.Fatal("invalid administrator password was accepted")
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Fatal("failed initialization left configuration files behind")
		}
	}
}

func TestRunInitializesConfigWithoutStartingServer(t *testing.T) {
	clearInitializationEnvironment(t)
	t.Setenv("GROK2API_ADMIN_PASSWORD", "test-only-password")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := Run([]string{"init-config", "--config", path}); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err != nil {
		t.Fatal(err)
	}
}

package cli

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chenyme/grok2api/backend/internal/infra/config"
	"gopkg.in/yaml.v3"
)

// initializeConfigFile creates only the secrets and bootstrap account; all
// other settings continue to use the application's current defaults.
func initializeConfigFile(path string, output io.Writer) error {
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return fmt.Errorf("config path is a directory: %s", path)
		}
		_, _ = fmt.Fprintf(output, "Using existing config: %s\n", path)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect config: %w", err)
	}

	username := strings.TrimSpace(os.Getenv("GROK2API_ADMIN_USERNAME"))
	if username == "" {
		username = "admin"
	}
	password := os.Getenv("GROK2API_ADMIN_PASSWORD")
	passwordSource := "GROK2API_ADMIN_PASSWORD"
	if password == "" && os.Getenv("ZEABUR") != "" {
		password = os.Getenv("PASSWORD")
		passwordSource = "Zeabur PASSWORD"
	}
	generatedPassword := password == ""
	if generatedPassword {
		value, err := randomBytes(24)
		if err != nil {
			return err
		}
		password = hex.EncodeToString(value)
	}
	if len(password) < 8 || len(password) > 72 {
		return errors.New("initial administrator password must contain 8 to 72 bytes")
	}
	jwt, err := randomBytes(32)
	if err != nil {
		return err
	}
	key, err := randomBytes(32)
	if err != nil {
		return err
	}
	document := struct {
		Secrets        config.Secrets              `yaml:"secrets"`
		BootstrapAdmin config.BootstrapAdminConfig `yaml:"bootstrapAdmin"`
	}{
		Secrets: config.Secrets{
			JWTSecret:               hex.EncodeToString(jwt),
			CredentialEncryptionKey: base64.StdEncoding.EncodeToString(key),
		},
		BootstrapAdmin: config.BootstrapAdminConfig{Username: username, Password: password},
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode initial config: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create initial config: %w", err)
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write initial config: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync initial config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close initial config: %w", err)
	}
	if _, err := config.Load(temporary.Name()); err != nil {
		return fmt.Errorf("validate initial config: %w", err)
	}
	// Link publishes a complete file atomically without replacing another
	// initializer's config or rotating the keys in an existing deployment.
	if err := os.Link(temporary.Name(), path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("publish initial config: %w", err)
	}
	_, _ = fmt.Fprintf(output, "Initialized config: %s\nInitial admin username: %s\n", path, username)
	if generatedPassword {
		_, _ = fmt.Fprintf(output, "Initial admin password: %s\n", password)
	} else {
		_, _ = fmt.Fprintf(output, "Initial admin password uses %s.\n", passwordSource)
	}
	return nil
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return nil, fmt.Errorf("generate initial secret: %w", err)
	}
	return value, nil
}

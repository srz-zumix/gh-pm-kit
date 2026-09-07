package lint

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the structure of the pm-kit configuration file.
type Config struct {
	Lint Options `yaml:"lint"`
}

// ConfigPaths are searched in order when no configuration file is given explicitly.
var ConfigPaths = []string{".github/pm-kit.yml", ".github/pm-kit.yaml"}

// LoadConfig reads the lint options from the configuration file at path.
func LoadConfig(path string) (*Options, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var config Config
	if err := decoder.Decode(&config); err != nil {
		if errors.Is(err, io.EOF) {
			return &Options{}, nil
		}
		return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
	}
	return &config.Lint, nil
}

// FindConfig loads the first configuration file found in ConfigPaths.
// It returns an empty path and nil options when no configuration file exists.
func FindConfig() (string, *Options, error) {
	for _, path := range ConfigPaths {
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return "", nil, fmt.Errorf("failed to check config file %q: %w", path, err)
		}
		opts, err := LoadConfig(path)
		if err != nil {
			return "", nil, err
		}
		return path, opts, nil
	}
	return "", nil, nil
}

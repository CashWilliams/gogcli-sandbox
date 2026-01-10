package config

import (
	"errors"
	"os"
	"path/filepath"
)

const (
	appConfigDirName  = "gogcli-sandbox"
	policyFileName    = "policy.json"
	configFileName    = "config.json"
	defaultSocketPath = "/run/gogcli-sandbox.sock"
)

func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if base == "" {
		return "", errors.New("config dir not available")
	}
	return filepath.Join(base, appConfigDirName), nil
}

func DefaultPolicyPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, policyFileName), nil
}

func DefaultConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func EnsurePolicyDir(path string) error {
	if path == "" {
		return errors.New("policy path is empty")
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "/" {
		return nil
	}
	return os.MkdirAll(dir, 0o700)
}

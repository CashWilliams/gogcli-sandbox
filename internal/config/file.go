package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

type FileConfig struct {
	Socket     string `json:"socket"`
	Policy     string `json:"policy"`
	GogPath    string `json:"gog_path"`
	GogAccount string `json:"gog_account"`
	Timeout    string `json:"timeout"`
	LogJSON    *bool  `json:"log_json"`
	Verbose    *bool  `json:"verbose"`
}

func DefaultFileConfig() FileConfig {
	policyPath, _ := DefaultPolicyPath()
	return FileConfig{
		Socket:  defaultSocketPath,
		Policy:  policyPath,
		GogPath: "gog",
		Timeout: (30 * time.Second).String(),
		LogJSON: boolPtr(true),
		Verbose: boolPtr(false),
	}
}

func LoadFile(path string, required bool) (*FileConfig, error) {
	if path == "" {
		if required {
			return nil, errors.New("config path is empty")
		}
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return nil, nil
		}
		return nil, err
	}
	var cfg FileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config json: %w", err)
	}
	return &cfg, nil
}

func WriteFile(path string, cfg FileConfig) error {
	if path == "" {
		return errors.New("config path is empty")
	}
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := EnsurePolicyDir(path); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o600)
}

func boolPtr(v bool) *bool {
	return &v
}

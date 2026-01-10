package config

import (
	"errors"
	"flag"
	"time"
)

type Config struct {
	ConfigPath string
	SocketPath string
	PolicyPath string
	GogPath    string
	GogAccount string
	Timeout    time.Duration
	LogJSON    bool
	Verbose    bool
}

func Load() (*Config, error) {
	defaultPolicyPath, _ := DefaultPolicyPath()
	defaultConfigPath, _ := DefaultConfigPath()

	cfg := &Config{
		ConfigPath: defaultConfigPath,
		SocketPath: defaultSocketPath,
		PolicyPath: defaultPolicyPath,
		GogPath:    "gog",
		Timeout:    30 * time.Second,
		LogJSON:    true,
		Verbose:    false,
	}

	flag.StringVar(&cfg.ConfigPath, "config", defaultConfigPath, "config file path (default: $XDG_CONFIG_HOME/gogcli-sandbox/config.json)")
	flag.StringVar(&cfg.SocketPath, "socket", cfg.SocketPath, "unix socket path")
	flag.StringVar(&cfg.PolicyPath, "policy", cfg.PolicyPath, "policy file path (default: $XDG_CONFIG_HOME/gogcli-sandbox/policy.json)")
	flag.StringVar(&cfg.GogPath, "gog-path", cfg.GogPath, "path to gog executable")
	flag.StringVar(&cfg.GogAccount, "gog-account", cfg.GogAccount, "gog account identifier (optional)")
	flag.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "gog execution timeout")
	flag.BoolVar(&cfg.LogJSON, "log-json", cfg.LogJSON, "emit JSON logs")
	flag.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "verbose logging (safe metadata only)")
	flag.Parse()

	explicit := map[string]bool{}
	flag.CommandLine.Visit(func(f *flag.Flag) {
		explicit[f.Name] = true
	})

	fileCfg, err := LoadFile(cfg.ConfigPath, explicit["config"])
	if err != nil {
		return nil, err
	}
	if fileCfg != nil {
		if !explicit["socket"] && fileCfg.Socket != "" {
			cfg.SocketPath = fileCfg.Socket
		}
		if !explicit["policy"] && fileCfg.Policy != "" {
			cfg.PolicyPath = fileCfg.Policy
		}
		if !explicit["gog-path"] && fileCfg.GogPath != "" {
			cfg.GogPath = fileCfg.GogPath
		}
		if !explicit["gog-account"] && fileCfg.GogAccount != "" {
			cfg.GogAccount = fileCfg.GogAccount
		}
		if !explicit["timeout"] && fileCfg.Timeout != "" {
			parsed, err := time.ParseDuration(fileCfg.Timeout)
			if err != nil {
				return nil, err
			}
			cfg.Timeout = parsed
		}
		if !explicit["log-json"] && fileCfg.LogJSON != nil {
			cfg.LogJSON = *fileCfg.LogJSON
		}
		if !explicit["verbose"] && fileCfg.Verbose != nil {
			cfg.Verbose = *fileCfg.Verbose
		}
	}

	if cfg.PolicyPath == "" {
		return nil, errors.New("policy path is required (set --policy or config file)")
	}
	if err := EnsurePolicyDir(cfg.PolicyPath); err != nil {
		return nil, err
	}
	return cfg, nil
}

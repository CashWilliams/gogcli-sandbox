package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gogcli-sandbox/internal/broker"
	"gogcli-sandbox/internal/config"
	"gogcli-sandbox/internal/gog"
	"gogcli-sandbox/internal/policy"
	"gogcli-sandbox/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	pol, err := policy.Load(cfg.PolicyPath)
	if err != nil {
		log.Fatalf("policy error: %v", err)
	}

	var logger broker.Logger
	if cfg.LogJSON {
		logger = broker.NewJSONLogger()
	} else {
		logger = broker.NewTextLogger()
	}

	runner := &gog.GogRunner{Path: cfg.GogPath, Account: cfg.GogAccount, Timeout: cfg.Timeout}
	pol.SetTimeZoneProvider(calendarTimeZoneProvider(runner))

	b := &broker.Broker{
		Policy:  pol,
		Runner:  runner,
		Logger:  logger,
		Verbose: cfg.Verbose,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Serve(ctx, cfg.SocketPath, b, logger); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func calendarTimeZoneProvider(runner gog.Runner) func(context.Context) (*time.Location, error) {
	return func(ctx context.Context) (*time.Location, error) {
		data, err := runner.Run(ctx, "calendar.list", map[string]interface{}{"max": 250})
		if err != nil {
			return nil, err
		}
		root, ok := data.(map[string]interface{})
		if !ok {
			return nil, errInvalidCalendarList
		}
		raw, ok := root["calendars"]
		if !ok {
			return nil, errInvalidCalendarList
		}
		items, ok := raw.([]interface{})
		if !ok {
			return nil, errInvalidCalendarList
		}
		timeZone := ""
		for _, item := range items {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			primary, _ := m["primary"].(bool)
			if !primary {
				continue
			}
			if tz, _ := m["timeZone"].(string); tz != "" {
				timeZone = tz
				break
			}
		}
		if timeZone == "" {
			for _, item := range items {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				if tz, _ := m["timeZone"].(string); tz != "" {
					timeZone = tz
					break
				}
			}
		}
		if timeZone == "" {
			return nil, errInvalidCalendarList
		}
		return time.LoadLocation(timeZone)
	}
}

var errInvalidCalendarList = logError("invalid calendar list response")

type logError string

func (e logError) Error() string { return string(e) }

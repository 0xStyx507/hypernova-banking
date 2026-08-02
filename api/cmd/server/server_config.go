package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

const (
	defaultAPIPort     = "8080"
	healthcheckCommand = "healthcheck"
	startupTimeout     = 15 * time.Second
)

func isHealthcheckCommand(args []string) bool {
	return len(args) > 1 && args[1] == healthcheckCommand
}

func apiPort(configuredPort string) string {
	if configuredPort == "" {
		return defaultAPIPort
	}
	return configuredPort
}

// resolveLedgerAddress converts a Compose DNS name into an IP because the
// native TigerBeetle client accepts numeric cluster addresses only.
func resolveLedgerAddress(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("invalid ledger address: %w", err)
	}
	if parsed := net.ParseIP(host); parsed != nil {
		return net.JoinHostPort(parsed.String(), port), nil
	}
	ips, err := net.LookupHost(host)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("resolve ledger host %q: %w", host, err)
	}
	return net.JoinHostPort(ips[0], port), nil
}

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func boolFromEnv(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

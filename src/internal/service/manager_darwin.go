//go:build darwin

package service

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
)

// LaunchdManager manages launchd services on macOS (§8.3, §8.4).
type LaunchdManager struct {
	logger *logging.Logger
}

// NewPlatformManager returns the launchd service manager.
func NewPlatformManager(logger *logging.Logger) Manager {
	return &LaunchdManager{logger: logger}
}

func (m *LaunchdManager) plistPath(name string) string {
	return fmt.Sprintf("/Library/LaunchDaemons/%s.plist", name)
}

// plistData holds the values for generating a launchd plist.
type plistData struct {
	Label   string
	Program string
	Args    []string
}

// generatePlist creates a launchd plist XML for the service.
func generatePlist(data plistData) ([]byte, error) {
	type plistDict struct {
		XMLName xml.Name `xml:"plist"`
		Version string   `xml:"version,attr"`
		Dict    struct {
			// We'll generate manually for clean output
		} `xml:"dict"`
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	sb.WriteString(`<plist version="1.0">` + "\n")
	sb.WriteString("<dict>\n")
	sb.WriteString("  <key>Label</key>\n")
	sb.WriteString(fmt.Sprintf("  <string>%s</string>\n", data.Label))
	sb.WriteString("  <key>ProgramArguments</key>\n")
	sb.WriteString("  <array>\n")
	sb.WriteString(fmt.Sprintf("    <string>%s</string>\n", data.Program))
	for _, arg := range data.Args {
		sb.WriteString(fmt.Sprintf("    <string>%s</string>\n", arg))
	}
	sb.WriteString("  </array>\n")
	sb.WriteString("  <key>RunAtLoad</key>\n")
	sb.WriteString("  <true/>\n")
	sb.WriteString("  <key>KeepAlive</key>\n")
	sb.WriteString("  <true/>\n")
	sb.WriteString("</dict>\n")
	sb.WriteString("</plist>\n")
	return []byte(sb.String()), nil
}

// Install creates the plist and loads the service. Idempotent (§8.4).
func (m *LaunchdManager) Install(ctx context.Context, opts Options) error {
	path := m.plistPath(opts.Name)

	content, err := generatePlist(plistData{
		Label:   opts.Name,
		Program: opts.BinaryPath,
		Args:    opts.Arguments,
	})
	if err != nil {
		return fmt.Errorf("generate plist: %w", err)
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write plist %q: %w", path, err)
	}

	// Load (bootstrap) the service — idempotent
	_ = m.launchctl(ctx, "bootout", "system/"+opts.Name)
	if err := m.launchctl(ctx, "bootstrap", "system", path); err != nil {
		return fmt.Errorf("bootstrap service %q: %w", opts.Name, err)
	}

	m.logger.Information(fmt.Sprintf("Service %q installed (plist: %s)", opts.Name, path))
	return nil
}

// Remove unloads and removes the plist. Idempotent (§8.4).
func (m *LaunchdManager) Remove(ctx context.Context, opts Options) error {
	path := m.plistPath(opts.Name)

	_ = m.launchctl(ctx, "bootout", "system/"+opts.Name)

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plist %q: %w", path, err)
	}

	m.logger.Information(fmt.Sprintf("Service %q removed", opts.Name))
	return nil
}

// Start starts the launchd service. Idempotent (§8.4).
func (m *LaunchdManager) Start(ctx context.Context, opts Options) error {
	if err := m.launchctl(ctx, "kickstart", "-k", "system/"+opts.Name); err != nil {
		return fmt.Errorf("start service %q: %w", opts.Name, err)
	}
	m.logger.Information(fmt.Sprintf("Service %q started", opts.Name))
	return nil
}

// Stop stops the launchd service. Idempotent (§8.4).
func (m *LaunchdManager) Stop(ctx context.Context, opts Options) error {
	if err := m.launchctl(ctx, "kill", "SIGTERM", "system/"+opts.Name); err != nil {
		return fmt.Errorf("stop service %q: %w", opts.Name, err)
	}
	m.logger.Information(fmt.Sprintf("Service %q stopped", opts.Name))
	return nil
}

func (m *LaunchdManager) launchctl(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "launchctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %s: %s (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}


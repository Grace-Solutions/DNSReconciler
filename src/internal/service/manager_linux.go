//go:build linux

package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
)

const unitTemplate = `[Unit]
Description={{.Description}}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart={{.BinaryPath}}{{range .Arguments}} {{.}}{{end}}
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
`

// SystemdManager manages systemd services on Linux (§8.3, §8.4).
type SystemdManager struct {
	logger *logging.Logger
}

// NewPlatformManager returns the systemd service manager.
func NewPlatformManager(logger *logging.Logger) Manager {
	return &SystemdManager{logger: logger}
}

func (m *SystemdManager) unitPath(name string) string {
	return fmt.Sprintf("/etc/systemd/system/%s.service", name)
}

// Install creates the systemd unit file and enables the service. Idempotent (§8.4).
func (m *SystemdManager) Install(ctx context.Context, opts Options) error {
	path := m.unitPath(opts.Name)

	tmpl, err := template.New("unit").Parse(unitTemplate)
	if err != nil {
		return fmt.Errorf("parse unit template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create unit file %q: %w", path, err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, opts); err != nil {
		return fmt.Errorf("write unit file %q: %w", path, err)
	}

	if err := m.systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if err := m.systemctl(ctx, "enable", opts.Name+".service"); err != nil {
		return err
	}

	m.logger.Information(fmt.Sprintf("Service %q installed (unit: %s)", opts.Name, path))
	return nil
}

// Uninstall disables and removes the systemd unit file. Idempotent (§8.4).
func (m *SystemdManager) Uninstall(ctx context.Context, opts Options) error {
	path := m.unitPath(opts.Name)

	_ = m.Stop(ctx, opts)
	_ = m.systemctl(ctx, "disable", opts.Name+".service")

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove unit file %q: %w", path, err)
	}
	_ = m.systemctl(ctx, "daemon-reload")

	m.logger.Information(fmt.Sprintf("Service %q uninstalled", opts.Name))
	return nil
}

// Start starts the systemd service. Idempotent (§8.4).
func (m *SystemdManager) Start(ctx context.Context, opts Options) error {
	if err := m.systemctl(ctx, "start", opts.Name+".service"); err != nil {
		return fmt.Errorf("start service %q: %w", opts.Name, err)
	}
	m.logger.Information(fmt.Sprintf("Service %q started", opts.Name))
	return nil
}

// Stop stops the systemd service. Idempotent (§8.4).
func (m *SystemdManager) Stop(ctx context.Context, opts Options) error {
	if err := m.systemctl(ctx, "stop", opts.Name+".service"); err != nil {
		return fmt.Errorf("stop service %q: %w", opts.Name, err)
	}
	m.logger.Information(fmt.Sprintf("Service %q stopped", opts.Name))
	return nil
}

func (m *SystemdManager) systemctl(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %s (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}


//go:build windows

package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/gracesolutions/dns-automatic-updater/internal/logging"
)

// WindowsManager manages Windows Services via sc.exe (§8.3, §8.4).
type WindowsManager struct {
	logger *logging.Logger
}

// NewPlatformManager returns the Windows service manager.
func NewPlatformManager(logger *logging.Logger) Manager {
	return &WindowsManager{logger: logger}
}

// Install registers the service. Idempotent: succeeds if already installed (§8.4).
func (m *WindowsManager) Install(ctx context.Context, opts Options) error {
	if m.serviceExists(ctx, opts.Name) {
		m.logger.Information(fmt.Sprintf("Service %q already installed, skipping", opts.Name))
		return nil
	}

	binPath := opts.BinaryPath
	if len(opts.Arguments) > 0 {
		binPath = binPath + " " + strings.Join(opts.Arguments, " ")
	}

	args := []string{"create", opts.Name,
		"binPath=", binPath,
		"start=", "auto",
		"DisplayName=", opts.DisplayName,
	}
	if opts.Description != "" {
		if err := m.runSC(ctx, "description", opts.Name, opts.Description); err != nil {
			m.logger.Warning(fmt.Sprintf("Failed to set description for %q: %s", opts.Name, err))
		}
	}

	if err := m.runSC(ctx, args...); err != nil {
		return fmt.Errorf("install service %q: %w", opts.Name, err)
	}
	m.logger.Information(fmt.Sprintf("Service %q installed successfully", opts.Name))
	return nil
}

// Uninstall unregisters the service. Idempotent: succeeds if not installed (§8.4).
func (m *WindowsManager) Uninstall(ctx context.Context, opts Options) error {
	if !m.serviceExists(ctx, opts.Name) {
		m.logger.Information(fmt.Sprintf("Service %q not found, nothing to uninstall", opts.Name))
		return nil
	}

	// Stop first if running
	_ = m.Stop(ctx, opts)

	if err := m.runSC(ctx, "delete", opts.Name); err != nil {
		return fmt.Errorf("uninstall service %q: %w", opts.Name, err)
	}
	m.logger.Information(fmt.Sprintf("Service %q uninstalled successfully", opts.Name))
	return nil
}

// Start starts the service. Idempotent: succeeds if already running (§8.4).
func (m *WindowsManager) Start(ctx context.Context, opts Options) error {
	if m.isRunning(ctx, opts.Name) {
		m.logger.Information(fmt.Sprintf("Service %q already running", opts.Name))
		return nil
	}

	if err := m.runSC(ctx, "start", opts.Name); err != nil {
		return fmt.Errorf("start service %q: %w", opts.Name, err)
	}
	m.logger.Information(fmt.Sprintf("Service %q started", opts.Name))
	return nil
}

// Stop stops the service. Idempotent: succeeds if already stopped (§8.4).
func (m *WindowsManager) Stop(ctx context.Context, opts Options) error {
	if !m.isRunning(ctx, opts.Name) {
		m.logger.Information(fmt.Sprintf("Service %q not running, nothing to stop", opts.Name))
		return nil
	}

	if err := m.runSC(ctx, "stop", opts.Name); err != nil {
		return fmt.Errorf("stop service %q: %w", opts.Name, err)
	}
	m.logger.Information(fmt.Sprintf("Service %q stopped", opts.Name))
	return nil
}

func (m *WindowsManager) serviceExists(ctx context.Context, name string) bool {
	err := m.runSC(ctx, "query", name)
	return err == nil
}

func (m *WindowsManager) isRunning(ctx context.Context, name string) bool {
	out, err := exec.CommandContext(ctx, "sc.exe", "query", name).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "RUNNING")
}

func (m *WindowsManager) runSC(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "sc.exe", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc.exe %s: %s (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}


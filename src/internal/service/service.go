package service

import "context"

type Action string

const (
	ActionInstall   Action = "install"
	ActionUninstall Action = "uninstall"
	ActionStart     Action = "start"
	ActionStop      Action = "stop"
	ActionInit      Action = "init"
)

type Options struct {
	Name        string
	DisplayName string
	Description string
	BinaryPath  string
	Arguments   []string
}

type Manager interface {
	Install(ctx context.Context, options Options) error
	Uninstall(ctx context.Context, options Options) error
	Start(ctx context.Context, options Options) error
	Stop(ctx context.Context, options Options) error
}

type UnsupportedManager struct{}

func NewUnsupportedManager() UnsupportedManager {
	return UnsupportedManager{}
}

func (UnsupportedManager) Install(context.Context, Options) error   { return ErrUnsupported }
func (UnsupportedManager) Uninstall(context.Context, Options) error { return ErrUnsupported }
func (UnsupportedManager) Start(context.Context, Options) error     { return ErrUnsupported }
func (UnsupportedManager) Stop(context.Context, Options) error      { return ErrUnsupported }
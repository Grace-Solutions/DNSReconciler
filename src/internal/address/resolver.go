package address

import (
	"context"

	"github.com/gracesolutions/dns-automatic-updater/internal/config"
	"github.com/gracesolutions/dns-automatic-updater/internal/runtimectx"
)

type Result struct {
	Source  config.AddressSource
	Address string
}

type Resolver interface {
	Resolve(ctx context.Context, runtimeSnapshot runtimectx.Snapshot, sources []config.AddressSource, ipFamily string) (Result, error)
}
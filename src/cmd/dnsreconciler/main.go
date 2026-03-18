package main

//go:generate goversioninfo -64 -icon=../../../resources/icons/dns-00001.ico -o resource_windows.syso

import (
	"os"

	"github.com/gracesolutions/dns-automatic-updater/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:], os.Stdout, os.Stderr))
}
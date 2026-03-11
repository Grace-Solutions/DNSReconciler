package main

import (
	"os"

	"github.com/gracesolutions/dns-automatic-updater/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:], os.Stdout, os.Stderr))
}
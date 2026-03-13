package main

import (
	"context"
	"os"

	"github.com/marian2js/ocpm/internal/app"
	"github.com/marian2js/ocpm/internal/version"
)

var (
	buildVersion = "dev"
	commit       = "none"
	date         = "unknown"
)

func main() {
	info := version.New(buildVersion, commit, date)
	code := app.Run(context.Background(), os.Stdout, os.Stderr, info, os.Args[1:])
	os.Exit(code)
}

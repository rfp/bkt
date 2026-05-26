package main

import (
	"os"
	"strings"
)

func init() {
	if !shouldUseAppEntrypoint(os.Args) {
		return
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal(err)
	}

	app := NewDefaultApp(cfg)
	if err := app.Run(os.Args[1:]); err != nil {
		fatal(err)
	}

	os.Exit(0)
}

func shouldUseAppEntrypoint(args []string) bool {
	if os.Getenv("BKT_USE_LEGACY_MAIN") == "1" {
		return false
	}
	if len(args) == 0 {
		return true
	}
	return !strings.HasSuffix(args[0], ".test")
}

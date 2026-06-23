package main

import (
	"flag"
	"os"

	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/pkg/migrate"
)

func main() {
	direction := flag.String("direction", "up", "migration direction: up, down, or force")
	version := flag.Int("version", 0, "target version for force")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	var runErr error
	switch *direction {
	case "up":
		runErr = migrate.Up(cfg.Database.URL, "migrations")
	case "down":
		runErr = migrate.Down(cfg.Database.URL, "migrations")
	case "force":
		if *version <= 0 {
			os.Stderr.WriteString("force requires -version > 0\n")
			os.Exit(1)
		}
		runErr = migrate.Force(cfg.Database.URL, "migrations", *version)
	default:
		os.Stderr.WriteString("direction must be up, down, or force\n")
		os.Exit(1)
	}

	if runErr != nil {
		panic(runErr)
	}
}

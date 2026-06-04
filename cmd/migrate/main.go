package main

import (
	"flag"
	"os"

	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/pkg/migrate"
)

func main() {
	direction := flag.String("direction", "up", "migration direction: up or down")
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
	default:
		os.Stderr.WriteString("direction must be up or down\n")
		os.Exit(1)
	}

	if runErr != nil {
		panic(runErr)
	}
}

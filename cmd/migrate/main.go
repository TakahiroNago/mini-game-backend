// Command migrate applies, reverts or inspects schema migrations using the
// SQL files embedded in the binary. Example:
//
//	go run ./cmd/migrate -cmd up
//	go run ./cmd/migrate -cmd down -steps 1
//	go run ./cmd/migrate -cmd version
//	go run ./cmd/migrate -cmd force -version 1   # repair a dirty state
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"

	"github.com/takahironago/mini-game-backend/internal/config"
	"github.com/takahironago/mini-game-backend/internal/database"
)

func main() {
	cmd := flag.String("cmd", "up", "up | down | version | force")
	steps := flag.Int("steps", 0, "number of migrations for down (0 = all)")
	ver := flag.Int("version", -1, "target version for force")
	flag.Parse()

	if err := run(*cmd, *steps, *ver); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(cmd string, steps, ver int) error {
	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	m, closeFn, err := database.NewMigrator(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer closeFn()

	switch cmd {
	case "up":
		err = m.Up()
	case "down":
		if steps > 0 {
			err = m.Steps(-steps)
		} else {
			err = m.Down()
		}
	case "version":
		v, dirty, verr := m.Version()
		if errors.Is(verr, migrate.ErrNilVersion) {
			fmt.Println("version: none")
			return nil
		}
		if verr != nil {
			return verr
		}
		fmt.Printf("version: %d dirty: %v\n", v, dirty)
		return nil
	case "force":
		if ver < 0 {
			return errors.New("-version is required for force")
		}
		err = m.Force(ver)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("no change")
		return nil
	}
	if err != nil {
		return err
	}
	v, dirty, _ := m.Version()
	fmt.Printf("done: version %d dirty: %v\n", v, dirty)
	return nil
}

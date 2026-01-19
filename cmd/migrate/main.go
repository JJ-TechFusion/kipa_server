package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	var (
		migrationsPath string
		databaseURL    string
		command        string
		steps          int
	)

	flag.StringVar(&migrationsPath, "path", "migrations", "Path to migrations directory")
	flag.StringVar(&databaseURL, "database", "", "Database URL (postgres://user:pass@host:port/dbname?sslmode=disable)")
	flag.StringVar(&command, "command", "up", "Migration command: up, down, drop, version, force, goto")
	flag.IntVar(&steps, "steps", 0, "Number of migrations to run (for up/down with specific count)")
	flag.Parse()

	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}

	if databaseURL == "" {
		host := getEnvOrDefault("DATABASE_HOST", "localhost")
		port := getEnvOrDefault("DATABASE_PORT", "5432")
		user := getEnvOrDefault("DATABASE_USER", "postgres")
		password := os.Getenv("DATABASE_PASSWORD")
		dbname := getEnvOrDefault("DATABASE_DBNAME", "kipa")
		sslmode := getEnvOrDefault("DATABASE_SSLMODE", "disable")

		databaseURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
			user, password, host, port, dbname, sslmode)
	}

	// Create migrate instance
	m, err := migrate.New(
		fmt.Sprintf("file://%s", migrationsPath),
		databaseURL,
	)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}
	defer m.Close()

	// Execute command
	switch command {
	case "up":
		if steps > 0 {
			err = m.Steps(steps)
		} else {
			err = m.Up()
		}
		if err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Migration up failed: %v", err)
		}
		fmt.Println("Migrations applied successfully")

	case "down":
		if steps > 0 {
			err = m.Steps(-steps)
		} else {
			err = m.Down()
		}
		if err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Migration down failed: %v", err)
		}
		fmt.Println("Migrations rolled back successfully")

	case "drop":
		err = m.Drop()
		if err != nil {
			log.Fatalf("Drop failed: %v", err)
		}
		fmt.Println("Database dropped successfully")

	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			log.Fatalf("Failed to get version: %v", err)
		}
		fmt.Printf("Version: %d, Dirty: %v\n", version, dirty)

	case "force":
		if steps == 0 {
			log.Fatal("Force requires -steps flag with version number")
		}
		err = m.Force(steps)
		if err != nil {
			log.Fatalf("Force failed: %v", err)
		}
		fmt.Printf("Forced to version %d\n", steps)

	case "goto":
		if steps == 0 {
			log.Fatal("Goto requires -steps flag with version number")
		}
		err = m.Migrate(uint(steps))
		if err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Goto failed: %v", err)
		}
		fmt.Printf("Migrated to version %d\n", steps)

	default:
		log.Fatalf("Unknown command: %s", command)
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

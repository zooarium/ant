//go:build ignore

package main

import (
	"context"
	"log"
	"os"

	"ant/ent/migrate"

	atlasmigrate "ariga.io/atlas/sql/migrate"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql/schema"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	ctx := context.Background()
	dir, err := atlasmigrate.NewLocalDir("ent/migrate/migrations")
	if err != nil {
		log.Fatalf("failed creating atlas migration directory: %v", err)
	}
	opts := []schema.MigrateOption{
		schema.WithDir(dir),
		schema.WithMigrationMode(schema.ModeReplay),
		schema.WithDialect(dialect.SQLite),
		schema.WithFormatter(atlasmigrate.DefaultFormatter),
	}
	if len(os.Args) != 2 {
		log.Fatalln("migration name must be provided as argument")
	}
	err = migrate.NamedDiff(ctx, "sqlite3://?mode=memory&cache=shared&_fk=1", os.Args[1], opts...)
	if err != nil {
		log.Fatalf("failed generating migration: %v", err)
	}
}

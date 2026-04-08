package main

import (
	"embed"
	"fmt"
	"os"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"

	"github.com/rolivape/planrod/internal/cli"
)

//go:embed all:migrations
var migrationsFS embed.FS

func init() {
	sqlite_vec.Auto()
}

func main() {
	root := cli.NewRootCmd(migrationsFS)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

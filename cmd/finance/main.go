// Command finance is the single Finance application binary.
//
//	finance migrate            apply embedded Goose PostgreSQL migrations
//	finance serve              run the HTTP server
//	finance seed initial-categories
//	                            seed the initial category set once
package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

// version is the baseline build identifier.
const version = "0.0.0-dev"

func execute(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: finance <migrate|serve|seed>")
	}
	switch args[0] {
	case "migrate":
		if len(args) != 1 {
			return fmt.Errorf("usage: finance migrate")
		}
		return migrateCommand(context.Background(), out, errOut)
	case "serve":
		if len(args) != 1 {
			return fmt.Errorf("usage: finance serve")
		}
		return serveCommand(context.Background(), out, errOut)
	case "seed":
		return seedCommand(context.Background(), args[1:], out, errOut)
	default:
		return fmt.Errorf("unknown command %q (want migrate, serve, or seed)", args[0])
	}
}

func main() {
	if err := execute(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

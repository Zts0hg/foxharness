package main

import (
	"context"
	"fmt"
	"github.com/Zts0hg/foxharness/internal/mcpcmd"
	"github.com/Zts0hg/foxharness/internal/mcpintegration"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func runMCP(ctx context.Context, home, work string, args []string, out io.Writer) error {
	if len(args) >= 2 && (args[0] == "--workdir" || args[0] == "-workdir") {
		var err error
		work, err = filepath.Abs(args[1])
		if err != nil {
			return err
		}
		args = args[2:]
	}
	command, err := mcpcmd.Parse(args)
	if err != nil {
		return err
	}
	result, err := mcpintegration.NewManager(home, work, nil).ExecuteMCP(ctx, command, func(_ context.Context, url string) error {
		_, err := fmt.Fprintln(out, "Open this URL to authorize:", url)
		return err
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, mcpcmd.Format(result))
	return err
}
func dispatchMCP(args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	work, err := os.Getwd()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runMCP(ctx, home, work, args, os.Stdout)
}

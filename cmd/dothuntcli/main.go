package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	root := newRootCmd(version)
	executed, err := root.ExecuteContextC(ctx)
	if err != nil {
		var ce *cliError
		if errors.As(err, &ce) {
			if ce.Err != nil && ce.Err.Error() != "" {
				fmt.Fprintln(os.Stderr, ce.Err.Error())
				fmt.Fprintln(os.Stderr)
			}
			if ce.ShowUsage && ce.Cmd != nil {
				_ = ce.Cmd.Usage()
			}
			return ce.Code
		}

		// If the user hit Ctrl-C, exit with a conventional SIGINT code.
		if errors.Is(err, context.Canceled) {
			return 130
		}

		if err.Error() != "" {
			fmt.Fprintln(os.Stderr, err.Error())
			fmt.Fprintln(os.Stderr)
		}
		if executed == nil {
			executed = root
		}
		_ = executed.Usage()
		return 2
	}
	return 0
}

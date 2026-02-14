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
	if err := root.ExecuteContext(ctx); err != nil {
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
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

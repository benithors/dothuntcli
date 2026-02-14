package main

import "github.com/spf13/cobra"

type cliError struct {
	Code      int
	Err       error
	ShowUsage bool
	Cmd       *cobra.Command
}

func (e *cliError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

var errExit0 = &cliError{Code: 0}

func usageErr(cmd *cobra.Command, err error) error {
	return &cliError{Code: 2, Err: err, ShowUsage: true, Cmd: cmd}
}

package main

import (
	"fmt"
	"os"

	"github.com/masterchef/masterchef/internal/cli"
)

type exitCoder interface {
	ExitCode() int
}

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if ec, ok := err.(exitCoder); ok {
			os.Exit(ec.ExitCode())
		}
		os.Exit(1)
	}
}

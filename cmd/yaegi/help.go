package main

import "fmt"

const usage = `Yaegi is a Go interpreter.

Usage:

    yaegi [command] [arguments]

The commands are:

    extract     generate a wrapper file from a source package
    help        print usage information
    run         execute a Go program from source
    test        execute test functions in a Go package

Use "yaegi help <command>" for more information about a command.

If no command is given or if the first argument is not a command, then
the run command is assumed.
`

func help(arg []string) error {
	var cmd string
	if len(arg) > 0 {
		cmd = arg[0]
	}

	switch cmd {
	case "extract":
		return extractCmd([]string{"-h"})
	case "", "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	case "run":
		return run([]string{"-h"})
	case "test":
		return fmt.Errorf("help: test not implemented")
	default:
		return fmt.Errorf("help: invalid yaegi command: %v", cmd)
	}
}

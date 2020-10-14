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
    version     print version

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
	case Extract:
		return extractCmd([]string{"-h"})
	case Help, "", "-h", "--help":
		fmt.Print(usage)
		return nil
	case Run:
		return run([]string{"-h"})
	case Test:
		return test([]string{"-h"})
	case Version:
		fmt.Println("Usage: yaegi version")
		return nil
	default:
		return fmt.Errorf("help: invalid yaegi command: %v", cmd)
	}
}

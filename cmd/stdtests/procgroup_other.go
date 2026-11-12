//go:build !unix

package main

import "os/exec"

// ownGroup does nothing: without process groups, a timeout kills the
// process only (the default Cancel of exec.CommandContext).
func ownGroup(cmd *exec.Cmd) {}

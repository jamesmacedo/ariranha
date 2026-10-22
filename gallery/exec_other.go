//go:build !windows

package gallery

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}

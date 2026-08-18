//go:build linux

package lhmstreamdeckplugin

import (
	"fmt"
	"log"
	"os/exec"
	"syscall"
)

// spawnCompanion starts the bundled companion binary (working directory is the
// plugin directory, see main). Pdeathsig ties its lifetime to the plugin's.
func spawnCompanion(port int) (func() bool, error) {
	cmd := exec.Command("./lhm-companion", "-port", fmt.Sprint(port))
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	log.Printf("spawned bundled lhm-companion on port %d (pid %d)\n", port, cmd.Process.Pid)
	return exitedFunc(cmd), nil
}

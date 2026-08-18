//go:build darwin

package lhmstreamdeckplugin

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
)

// Darwin has no Pdeathsig, so the child cannot be tied to the plugin's lifetime
// by the kernel. Instead we track spawned companions and kill them when the
// Stream Deck app signals us to quit. A companion that outlives the plugin
// anyway is harmless: ensureOnce probes the port first and reuses whatever is
// already listening rather than spawning a duplicate.
var (
	spawnedMu   sync.Mutex
	spawnedPIDs []int
	reapOnce    sync.Once
)

func reapCompanionsOnExit() {
	reapOnce.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-ch
			killSpawnedCompanions()
			os.Exit(0)
		}()
	})
}

func killSpawnedCompanions() {
	spawnedMu.Lock()
	pids := append([]int(nil), spawnedPIDs...)
	spawnedPIDs = nil
	spawnedMu.Unlock()
	for _, pid := range pids {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Signal(syscall.SIGTERM)
		}
	}
}

// spawnCompanion starts the bundled companion binary (working directory is the
// plugin directory, see main).
func spawnCompanion(port int) (func() bool, error) {
	reapCompanionsOnExit()

	cmd := exec.Command("./lhm-companion", "-port", fmt.Sprint(port))
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	log.Printf("spawned bundled lhm-companion on port %d (pid %d)\n", port, pid)

	spawnedMu.Lock()
	spawnedPIDs = append(spawnedPIDs, pid)
	spawnedMu.Unlock()

	return exitedFunc(cmd), nil
}

//go:build linux || darwin

package lhmstreamdeckplugin

import "os/exec"

// exitedFunc reaps cmd in the background and returns a non-blocking predicate
// reporting whether the child has already exited.
func exitedFunc(cmd *exec.Cmd) func() bool {
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	return func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}
}

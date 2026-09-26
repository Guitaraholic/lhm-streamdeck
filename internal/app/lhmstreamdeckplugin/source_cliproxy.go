package lhmstreamdeckplugin

import "github.com/moeilijk/lhm-streamdeck/internal/cliproxy"

// startCLIProxySource wires rt.hw to a CLIProxyAPI management API. Like
// SparkDash this never spawns lhm-companion: the proxy already owns the
// account sessions and is reached over HTTP from the plugin.
func startCLIProxySource(rt *sourceRuntime) error {
	rt.hw = cliproxy.NewService(rt.profile.Host, rt.profile.Port, rt.profile.ManagementKey)
	return nil
}

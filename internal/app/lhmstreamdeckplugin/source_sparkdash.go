package lhmstreamdeckplugin

import "github.com/moeilijk/lhm-streamdeck/internal/sparkdash"

// startSparkDashSource wires rt.hw to a SparkDash dashboard. Unlike LHM
// sources this never spawns lhm-companion: SparkDash already owns the LLM
// probes and is reached over HTTP from the plugin.
func startSparkDashSource(rt *sourceRuntime) error {
	rt.hw = sparkdash.NewService(rt.profile.Host, rt.profile.Port, rt.profile.SparkID)
	return nil
}

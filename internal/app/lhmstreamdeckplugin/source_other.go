//go:build !linux && !darwin

package lhmstreamdeckplugin

func startCompanionSource(rt *sourceRuntime) error { return nil }

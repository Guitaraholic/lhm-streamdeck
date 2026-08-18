package lhmstreamdeckplugin

import "testing"

// The macOS companion invents its own sensor IDs, so pin the categories they
// classify into. sensorCategory is an ordered switch and the CPU arm runs
// first; a GPU sensor whose name happened to contain a CPU token would be
// filed under CPU and vanish from the GPU filter in the property inspector.
func TestSensorCategoryForMacCompanion(t *testing.T) {
	cases := []struct{ id, name, want string }{
		{"/apple/cpu/0", "Apple M4 Pro", "cpu"},
		{"/apple/gpu/0", "Apple M4 Pro GPU", "gpu"},
		{"/ram", "Generic Memory", "memory"},
		// lhm-companion on the Linux/DGX hosts
		{"/gpu-nvidia/0", "NVIDIA GB10", "gpu"},
		{"/amdcpu/0", "AMD Ryzen 9 7950X", "cpu"},
	}
	for _, c := range cases {
		if got := sensorCategory(c.id, c.name); got != c.want {
			t.Errorf("sensorCategory(%q, %q) = %q, want %q", c.id, c.name, got, c.want)
		}
	}
}

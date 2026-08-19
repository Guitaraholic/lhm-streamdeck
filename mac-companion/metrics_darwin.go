//go:build darwin

package main

/*
#include <mach/mach.h>
#include <mach/mach_host.h>
#include <mach/processor_info.h>
#include <sys/sysctl.h>
#include <stdlib.h>

static int cpu_ticks(natural_t *ncpu_out, unsigned int *buf, int buflen) {
	processor_info_array_t info;
	mach_msg_type_number_t cnt;
	natural_t ncpu;
	kern_return_t kr = host_processor_info(mach_host_self(),
		PROCESSOR_CPU_LOAD_INFO, &ncpu, &info, &cnt);
	if (kr != KERN_SUCCESS) return -1;
	int n = (int)ncpu * CPU_STATE_MAX;
	if (n > buflen) n = buflen;
	for (int i = 0; i < n; i++) buf[i] = ((integer_t *)info)[i];
	*ncpu_out = ncpu;
	vm_deallocate(mach_task_self(), (vm_address_t)info, cnt * sizeof(integer_t));
	return n;
}

static int vm_info(unsigned long long *active, unsigned long long *wired,
                   unsigned long long *compressed, unsigned long long *pagesize) {
	vm_statistics64_data_t st;
	mach_msg_type_number_t count = HOST_VM_INFO64_COUNT;
	if (host_statistics64(mach_host_self(), HOST_VM_INFO64,
			(host_info64_t)&st, &count) != KERN_SUCCESS) return -1;
	vm_size_t ps = 0;
	host_page_size(mach_host_self(), &ps);
	*active = st.active_count;
	*wired = st.wire_count;
	*compressed = st.compressor_page_count;
	*pagesize = (unsigned long long)ps;
	return 0;
}
*/
import "C"

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

const cpuStateMax = C.CPU_STATE_MAX

// cpuSampler converts cumulative mach CPU tick counters into a busy percentage
// by differencing successive samples.
type cpuSampler struct {
	mu   sync.Mutex
	prev []uint32
}

func (c *cpuSampler) sample() (busyPct float64, perCore []float64, ok bool) {
	buf := make([]C.uint, 256*cpuStateMax)
	var ncpu C.natural_t
	n := C.cpu_ticks(&ncpu, (*C.uint)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
	if n <= 0 {
		return 0, nil, false
	}
	cur := make([]uint32, int(n))
	for i := range cur {
		cur[i] = uint32(buf[i])
	}

	c.mu.Lock()
	prev := c.prev
	c.prev = cur
	c.mu.Unlock()

	if len(prev) != len(cur) {
		return 0, nil, false // first sample: no delta yet
	}

	cores := int(ncpu)
	perCore = make([]float64, 0, cores)
	var totBusy, totAll float64
	for i := 0; i < cores; i++ {
		o := i * cpuStateMax
		user := float64(cur[o+C.CPU_STATE_USER] - prev[o+C.CPU_STATE_USER])
		sys := float64(cur[o+C.CPU_STATE_SYSTEM] - prev[o+C.CPU_STATE_SYSTEM])
		nice := float64(cur[o+C.CPU_STATE_NICE] - prev[o+C.CPU_STATE_NICE])
		idle := float64(cur[o+C.CPU_STATE_IDLE] - prev[o+C.CPU_STATE_IDLE])
		busy := user + sys + nice
		all := busy + idle
		if all > 0 {
			perCore = append(perCore, busy/all*100)
		} else {
			perCore = append(perCore, 0)
		}
		totBusy += busy
		totAll += all
	}
	if totAll <= 0 {
		return 0, perCore, false
	}
	return totBusy / totAll * 100, perCore, true
}

// memInfo reports used/total physical memory in bytes, matching the
// "memory pressure" notion Activity Monitor shows (active+wired+compressed).
func memInfo() (used, total uint64, ok bool) {
	var active, wired, compressed, pagesize C.ulonglong
	if C.vm_info(&active, &wired, &compressed, &pagesize) != 0 {
		return 0, 0, false
	}
	used = uint64(active+wired+compressed) * uint64(pagesize)
	return used, uint64(sysctlUint64("hw.memsize")), true
}

func sysctlUint64(name string) uint64 {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	return v
}

func sysctlString(name string) string {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

var (
	reDeviceUtil = regexp.MustCompile(`"Device Utilization %"=(\d+)`)
	reRenderUtil = regexp.MustCompile(`"Renderer Utilization %"=(\d+)`)
	reTilerUtil  = regexp.MustCompile(`"Tiler Utilization %"=(\d+)`)
	reInUseMem   = regexp.MustCompile(`"In use system memory"=(\d+)`)
	reAllocMem   = regexp.MustCompile(`"Alloc system memory"=(\d+)`)
)

type gpuStats struct {
	deviceUtil, rendererUtil, tilerUtil float64
	inUseMem, allocMem                  uint64
	ok                                  bool
}

// gpuInfo reads Apple Silicon GPU counters from IOKit via ioreg. No sudo needed.
// Multiple accelerator nodes can appear; we take the maximum of each counter.
func gpuInfo() gpuStats {
	out, err := exec.Command("ioreg", "-r", "-d", "1", "-c", "IOAccelerator", "-w", "0").Output()
	if err != nil {
		return gpuStats{}
	}
	s := string(out)
	maxOf := func(re *regexp.Regexp) (float64, bool) {
		m := re.FindAllStringSubmatch(s, -1)
		if len(m) == 0 {
			return 0, false
		}
		best := 0.0
		for _, g := range m {
			if v, err := strconv.ParseFloat(g[1], 64); err == nil && v > best {
				best = v
			}
		}
		return best, true
	}
	g := gpuStats{}
	var found bool
	if v, ok := maxOf(reDeviceUtil); ok {
		g.deviceUtil, found = v, true
	}
	if v, ok := maxOf(reRenderUtil); ok {
		g.rendererUtil = v
	}
	if v, ok := maxOf(reTilerUtil); ok {
		g.tilerUtil = v
	}
	if v, ok := maxOf(reInUseMem); ok {
		g.inUseMem = uint64(v)
	}
	if v, ok := maxOf(reAllocMem); ok {
		g.allocMem = uint64(v)
	}
	g.ok = found
	return g
}

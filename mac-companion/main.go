// lhm-companion-mac serves macOS system metrics as /data.json in Libre Hardware
// Monitor format, so any LHM-compatible client (including the lhm-streamdeck
// plugin) can consume a Mac exactly like a Windows or Linux host.
//
// Deliberate omission: Apple Silicon CPU/GPU die temperatures require SMC
// access that macOS no longer exposes to unprivileged processes, so no
// Temperatures section is emitted rather than reporting fabricated values.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

const version = "1.0.0"

// node mirrors the Libre Hardware Monitor /data.json tree exactly.
type node struct {
	Text     string `json:"Text"`
	Min      string `json:"Min"`
	Value    string `json:"Value"`
	Max      string `json:"Max"`
	SensorID string `json:"SensorId"`
	Type     string `json:"Type"`
	ImageURL string `json:"ImageURL"`
	Children []node `json:"Children"`
}

func leaf(text, value, sensorID, typ string) node {
	return node{Text: text, Value: value, SensorID: sensorID, Type: typ, Children: []node{}}
}

func branch(text string, children ...node) node {
	return node{Text: text, Children: children}
}

func pct(v float64) string   { return fmt.Sprintf("%.1f %%", v) }
func gib(b uint64) string    { return fmt.Sprintf("%.1f GB", float64(b)/(1<<30)) }

type collector struct {
	cpu      cpuSampler
	mu       sync.RWMutex
	snapshot node
	ready    bool
	hostName string
	cpuName  string
}

func (c *collector) refresh() {
	root := branch("Sensor")

	// --- CPU -------------------------------------------------------------
	cpuChildren := []node{}
	if busy, perCore, ok := c.cpu.sample(); ok {
		loads := []node{leaf("CPU Total", pct(busy), "/apple/cpu/0/load/0", "Load")}
		for i, v := range perCore {
			loads = append(loads, leaf(fmt.Sprintf("CPU Core #%d", i+1), pct(v),
				fmt.Sprintf("/apple/cpu/0/load/%d", i+1), "Load"))
		}
		cpuChildren = append(cpuChildren, branch("Load", loads...))
	}
	if len(cpuChildren) > 0 {
		root.Children = append(root.Children, branch(c.cpuName, cpuChildren...))
	}

	// --- GPU -------------------------------------------------------------
	if g := gpuInfo(); g.ok {
		loads := []node{leaf("GPU Core", pct(g.deviceUtil), "/apple/gpu/0/load/0", "Load")}
		if g.rendererUtil > 0 || g.tilerUtil > 0 {
			loads = append(loads,
				leaf("GPU Renderer", pct(g.rendererUtil), "/apple/gpu/0/load/1", "Load"),
				leaf("GPU Tiler", pct(g.tilerUtil), "/apple/gpu/0/load/2", "Load"))
		}
		gpuChildren := []node{branch("Load", loads...)}
		if g.inUseMem > 0 || g.allocMem > 0 {
			gpuChildren = append(gpuChildren, branch("Data",
				leaf("GPU Memory Used", gib(g.inUseMem), "/apple/gpu/0/data/0", "Data"),
				leaf("GPU Memory Allocated", gib(g.allocMem), "/apple/gpu/0/data/1", "Data")))
		}
		root.Children = append(root.Children, branch(c.cpuName+" GPU", gpuChildren...))
	}

	// --- Memory ----------------------------------------------------------
	if used, total, ok := memInfo(); ok && total > 0 {
		usedPct := float64(used) / float64(total) * 100
		root.Children = append(root.Children, branch("Generic Memory",
			branch("Load",
				leaf("Memory", pct(usedPct), "/ram/load/0", "Load")),
			branch("Data",
				leaf("Memory Used", gib(used), "/ram/data/0", "Data"),
				leaf("Memory Available", gib(total-used), "/ram/data/1", "Data"),
				leaf("Memory Total", gib(total), "/ram/data/2", "Data"))))
	}

	c.mu.Lock()
	c.snapshot = root
	c.ready = true
	c.mu.Unlock()
}

func main() {
	port := flag.Int("port", envPort(), "HTTP port to listen on")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("lhm-companion-mac", version)
		return
	}

	host, _ := os.Hostname()
	c := &collector{hostName: host, cpuName: sysctlString("machdep.cpu.brand_string")}
	if c.cpuName == "" {
		c.cpuName = "Apple Silicon"
	}

	// Prime the tick counters before serving. CPU percentages are a delta
	// between two samples, so a single refresh yields no CPU section at all --
	// clients polling in that window would ask for a sensor that is missing
	// from the tree. Sample twice up front so the first response is complete.
	c.cpu.sample()
	time.Sleep(250 * time.Millisecond)
	c.refresh()
	go func() {
		for range time.Tick(time.Second) {
			c.refresh()
		}
	}()

	http.HandleFunc("/data.json", func(w http.ResponseWriter, r *http.Request) {
		c.mu.RLock()
		snap, ready := c.snapshot, c.ready
		c.mu.RUnlock()
		if !ready {
			http.Error(w, "collecting", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("lhm-companion-mac %s listening on %s (host=%s cpu=%s)", version, addr, host, c.cpuName)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func envPort() int {
	if v := os.Getenv("LHM_PORT"); v != "" {
		var p int
		if _, err := fmt.Sscanf(v, "%d", &p); err == nil && p > 0 && p < 65536 {
			return p
		}
	}
	return 8085
}

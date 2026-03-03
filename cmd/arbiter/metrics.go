package main

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var startTime = time.Now()

type InternalStats struct {
	CPUUser    float64
	CPUSys     float64
	MemAlloc   uint64
	MemSys     uint64
	MemRSS     uint64
	Goroutines int
}

// getSystemMetrics collects hardware and runtime information.
func getSystemMetrics() InternalStats {
	var s InternalStats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	s.MemAlloc = m.Alloc
	s.MemSys = m.Sys
	s.Goroutines = runtime.NumGoroutine()

	// Linux specific: Read process stats for CPU and RSS
	data, err := os.ReadFile("/proc/self/stat")
	if err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 23 {
			// Parsing CPU ticks (Field 14 and 15)
			utime, _ := strconv.ParseFloat(fields[13], 64)
			stime, _ := strconv.ParseFloat(fields[14], 64)
			
			// Corrected: strconv.ParseUint requires (string, base, bitSize)
			rss, _ := strconv.ParseUint(fields[23], 10, 64)

			s.CPUUser = utime / 100.0 // Assuming 100Hz clock ticks
			s.CPUSys = stime / 100.0
			s.MemRSS = rss * uint64(os.Getpagesize())
		}
	}
	return s
}

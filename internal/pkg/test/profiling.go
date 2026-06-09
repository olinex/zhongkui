package test

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/google/pprof/profile"
)

type ProfilingConfig struct {
	CPU       bool
	Heap      bool
	Goroutine bool
	Mutex     bool
	Block     bool
	TopN      int
}

func DefaultProfilingConfig() ProfilingConfig {
	return ProfilingConfig{
		CPU:       true,
		Heap:      true,
		Goroutine: true,
		Mutex:     true,
		Block:     true,
		TopN:      10,
	}
}

type ProfileEntry struct {
	Function string
	FileLine string
	Value    int64
	Percent  float64
}

type ProfileResult struct {
	CPUProfilePath  string
	HeapProfilePath string
	GoroutinePath   string
	MutexPath       string
	BlockPath       string

	TopCPU          []ProfileEntry
	TopAllocBytes   []ProfileEntry
	TopAllocObjects []ProfileEntry

	GoroutineCount   int
	MemStats         runtime.MemStats
	MutexContentions int64
	MutexDelayTotal  time.Duration
}

func newProfileOutputDir(baseDir string) string {
	timestamp := time.Now().Format("20060102_150405")
	dir := filepath.Join(baseDir, fmt.Sprintf("profile_%s", timestamp))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return baseDir
	}
	return dir
}

func safeFileName(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, name)
}

func collectProfiles(benchName string, outputDir string, cfg ProfilingConfig) *ProfileResult {
	result := &ProfileResult{}

	runtime.GC()
	runtime.ReadMemStats(&result.MemStats)

	if cfg.Mutex || cfg.Block {
		runtime.SetMutexProfileFraction(1)
		runtime.SetBlockProfileRate(1)
	}

	if cfg.CPU {
		safe := safeFileName(benchName)
		cpuPath := filepath.Join(outputDir, fmt.Sprintf("%s_cpu.pprof", safe))
		f, err := os.Create(cpuPath)
		if err == nil {
			if err := pprof.StartCPUProfile(f); err != nil {
				f.Close()
				os.Remove(cpuPath)
			}
			result.CPUProfilePath = cpuPath
		}
	}

	return result
}

func finalizeProfiles(result *ProfileResult, cfg ProfilingConfig) {
	if result == nil {
		return
	}

	if result.CPUProfilePath != "" {
		pprof.StopCPUProfile()
		entries, err := parseCPUProfile(result.CPUProfilePath, cfg.TopN)
		if err == nil {
			result.TopCPU = entries
		}
	}

	safeDir := filepath.Dir(result.CPUProfilePath)
	if safeDir == "" || safeDir == "." {
		safeDir = "."
	}
	safe := ""
	if result.CPUProfilePath != "" {
		safe = safeFileName(strings.TrimSuffix(
			strings.TrimSuffix(filepath.Base(result.CPUProfilePath), "_cpu.pprof"),
			".pprof",
		))
	}
	if safe == "" {
		safe = "bench"
	}

	if cfg.Heap {
		heapPath := filepath.Join(safeDir, fmt.Sprintf("%s_heap.pprof", safe))
		f, err := os.Create(heapPath)
		if err == nil {
			runtime.GC()
			if err := writeProfile("heap", f, 0); err == nil {
				result.HeapProfilePath = heapPath
			}
			if result.HeapProfilePath == "" {
				f.Close()
				os.Remove(heapPath)
			}
		}
		if result.HeapProfilePath != "" {
			entries, err := parseHeapProfile(result.HeapProfilePath, cfg.TopN)
			if err == nil {
				result.TopAllocBytes = entries
			}
			objEntries, err := parseHeapProfileObjects(result.HeapProfilePath, cfg.TopN)
			if err == nil {
				result.TopAllocObjects = objEntries
			}
		}
	}

	if cfg.Goroutine {
		goroPath := filepath.Join(safeDir, fmt.Sprintf("%s_goroutine.pprof", safe))
		f, err := os.Create(goroPath)
		if err == nil {
			if err := writeProfile("goroutine", f, 2); err == nil {
				result.GoroutinePath = goroPath
				result.GoroutineCount = countGoroutines(goroPath)
			}
			if result.GoroutinePath == "" {
				f.Close()
				os.Remove(goroPath)
			}
		}
	}

	if cfg.Mutex {
		mutexPath := filepath.Join(safeDir, fmt.Sprintf("%s_mutex.pprof", safe))
		f, err := os.Create(mutexPath)
		if err == nil {
			if err := writeProfile("mutex", f, 0); err == nil {
				result.MutexPath = mutexPath
			}
			if result.MutexPath == "" {
				f.Close()
				os.Remove(mutexPath)
			}
		}
		if result.MutexPath != "" {
			contentions, delay := parseMutexProfile(result.MutexPath)
			result.MutexContentions = contentions
			result.MutexDelayTotal = delay
		}
	}

	if cfg.Block {
		blockPath := filepath.Join(safeDir, fmt.Sprintf("%s_block.pprof", safe))
		f, err := os.Create(blockPath)
		if err == nil {
			if err := writeProfile("block", f, 0); err == nil {
				result.BlockPath = blockPath
			}
			if result.BlockPath == "" {
				f.Close()
				os.Remove(blockPath)
			}
		}
	}

	runtime.SetMutexProfileFraction(0)
	runtime.SetBlockProfileRate(0)

	runtime.GC()
	runtime.ReadMemStats(&result.MemStats)
}

func writeProfile(name string, w *os.File, debug int) error {
	defer w.Close()
	p := pprof.Lookup(name)
	if p == nil {
		return fmt.Errorf("profile %s not available", name)
	}
	return p.WriteTo(w, debug)
}

func countGoroutines(path string) int {
	data, err := readProfileFile(path)
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "goroutine ") {
			count++
		}
	}
	return count
}

func readProfileFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data, nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parseCPUProfile(path string, topN int) ([]ProfileEntry, error) {
	data, err := readProfileFile(path)
	if err != nil {
		return nil, err
	}

	p, err := profile.ParseData(data)
	if err != nil {
		return nil, err
	}

	cumByFunc := make(map[string]struct {
		cum int64
		loc *profile.Location
	})
	for _, s := range p.Sample {
		if len(s.Location) == 0 {
			continue
		}
		leaf := s.Location[len(s.Location)-1]
		cum := s.Value[0]
		for _, line := range leaf.Line {
			if line.Function != nil {
				name := line.Function.Name
				entry := cumByFunc[name]
				entry.cum += cum
				entry.loc = leaf
				cumByFunc[name] = entry
				break
			}
		}
	}

	totalValue := int64(0)
	for _, v := range cumByFunc {
		totalValue += v.cum
	}

	type funcEntry struct {
		name  string
		file  string
		line  int64
		value int64
	}

	entries := make([]funcEntry, 0, len(cumByFunc))
	for name, info := range cumByFunc {
		entry := funcEntry{name: name, value: info.cum}
		for _, loc := range p.Location {
			if loc == info.loc {
				for _, line := range loc.Line {
					if line.Function != nil && line.Function.Name == name {
						entry.file = line.Function.Filename
						entry.line = line.Line
						break
					}
				}
				break
			}
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].value > entries[j].value
	})

	if topN <= 0 {
		topN = 10
	}
	if len(entries) > topN {
		entries = entries[:topN]
	}

	result := make([]ProfileEntry, 0, len(entries))
	for _, e := range entries {
		percent := 0.0
		if totalValue > 0 {
			percent = float64(e.value) / float64(totalValue) * 100
		}
		fileLine := e.file
		if e.line > 0 {
			fileLine = fmt.Sprintf("%s:%d", e.file, e.line)
		}
		result = append(result, ProfileEntry{
			Function: e.name,
			FileLine: fileLine,
			Value:    e.value,
			Percent:  percent,
		})
	}

	return result, nil
}

func parseHeapProfile(path string, topN int) ([]ProfileEntry, error) {
	data, err := readProfileFile(path)
	if err != nil {
		return nil, err
	}

	p, err := profile.ParseData(data)
	if err != nil {
		return nil, err
	}

	allocBytesByFunc := make(map[string]int64)
	totalBytes := int64(0)

	for _, s := range p.Sample {
		if len(s.Value) < 2 {
			continue
		}
		allocBytes := s.Value[0]
		if allocBytes == 0 {
			continue
		}

		var funcName string
		var fileName string

		for _, loc := range s.Location {
			for _, line := range loc.Line {
				if line.Function != nil && funcName == "" {
					funcName = line.Function.Name
					fileName = line.Function.Filename
				}
			}
		}

		if funcName != "" {
			key := funcName
			if fileName != "" {
				key = funcName + "@" + fileName
			}
			allocBytesByFunc[key] += allocBytes
			totalBytes += allocBytes
		}
	}

	type allocEntry struct {
		name  string
		value int64
	}
	entries := make([]allocEntry, 0, len(allocBytesByFunc))
	for key, val := range allocBytesByFunc {
		entries = append(entries, allocEntry{name: key, value: val})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].value > entries[j].value
	})

	if topN <= 0 {
		topN = 10
	}
	if len(entries) > topN {
		entries = entries[:topN]
	}

	result := make([]ProfileEntry, 0, len(entries))
	for _, e := range entries {
		percent := 0.0
		if totalBytes > 0 {
			percent = float64(e.value) / float64(totalBytes) * 100
		}
		result = append(result, ProfileEntry{
			Function: e.name,
			Value:    e.value,
			Percent:  percent,
		})
	}

	return result, nil
}

func parseHeapProfileObjects(path string, topN int) ([]ProfileEntry, error) {
	data, err := readProfileFile(path)
	if err != nil {
		return nil, err
	}

	p, err := profile.ParseData(data)
	if err != nil {
		return nil, err
	}

	allocObjByFunc := make(map[string]int64)
	totalObjs := int64(0)

	for _, s := range p.Sample {
		if len(s.Value) < 2 {
			continue
		}
		allocObjs := s.Value[1]
		if allocObjs == 0 {
			continue
		}

		var funcName string
		for _, loc := range s.Location {
			for _, line := range loc.Line {
				if line.Function != nil && funcName == "" {
					funcName = line.Function.Name
				}
			}
		}

		if funcName != "" {
			allocObjByFunc[funcName] += allocObjs
			totalObjs += allocObjs
		}
	}

	type allocEntry struct {
		name  string
		value int64
	}
	entries := make([]allocEntry, 0, len(allocObjByFunc))
	for key, val := range allocObjByFunc {
		entries = append(entries, allocEntry{name: key, value: val})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].value > entries[j].value
	})

	if topN <= 0 {
		topN = 10
	}
	if len(entries) > topN {
		entries = entries[:topN]
	}

	result := make([]ProfileEntry, 0, len(entries))
	for _, e := range entries {
		percent := 0.0
		if totalObjs > 0 {
			percent = float64(e.value) / float64(totalObjs) * 100
		}
		result = append(result, ProfileEntry{
			Function: e.name,
			Value:    e.value,
			Percent:  percent,
		})
	}

	return result, nil
}

func parseMutexProfile(path string) (contentions int64, delay time.Duration) {
	data, err := readProfileFile(path)
	if err != nil {
		return 0, 0
	}

	p, err := profile.ParseData(data)
	if err != nil {
		return 0, 0
	}

	for _, s := range p.Sample {
		if len(s.Value) >= 2 {
			contentions += s.Value[0]
			delay += time.Duration(s.Value[1])
		}
	}
	return contentions, delay
}

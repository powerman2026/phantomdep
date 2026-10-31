// Command phantomdep is the orchestrator for the PhantomDep pipeline.
//
// Two modes:
//
//	phantomdep scan <path>     headless, prints JSON to stdout
//	phantomdep                 HTTP server + static UI on 127.0.0.1:8787
//
// The orchestrator is the only component allowed to know about all three
// stages; see .kiro/steering/structure.md rule 3.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"phantomdep/internal/analyze"
	"phantomdep/internal/intel"
	"phantomdep/internal/model"
	"phantomdep/internal/scan"
)

const version = "1.0.0"

type server struct {
	root       string // absolute repository root, also the default allowlist
	allow      []string
	webDir     string
	intelAddr  string
	analyserAt string
}

func main() {
	log.SetFlags(0)
	if len(os.Args) > 1 && os.Args[1] == "scan" {
		runCLI(os.Args[2:])
		return
	}
	runServer()
}

// ---------------------------------------------------------------- CLI mode --

func runCLI(args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	root := fs.String("root", ".", "repository root (holds web/, analyzer-rs/, intel-py/)")
	intelAddr := fs.String("intel", intel.DefaultAddr, "python intel service address")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: phantomdep scan [-root DIR] [-intel ADDR] <path>")
	}
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(2)
	}

	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		log.Fatalf("root: %v", err)
	}
	s := &server{root: rootAbs, webDir: filepath.Join(rootAbs, "web"), intelAddr: *intelAddr}
	// The CLI keeps the same allowlist discipline as the server so that a path
	// which works headlessly also works from the browser.
	s.allow = append(s.allow, rootAbs)
	if abs, err := filepath.Abs(fs.Arg(0)); err == nil {
		if resolved, rerr := filepath.EvalSymlinks(abs); rerr == nil {
			abs = resolved
		}
		s.allow = append(s.allow, abs)
	}

	res, code, err := s.scanPath(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(code)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		log.Fatalf("encode: %v", err)
	}
}

// -------------------------------------------------------------- HTTP mode --

func runServer() {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address (loopback by default)")
	root := flag.String("root", ".", "repository root")
	allow := flag.String("allow", "", "extra comma-separated directories that may be scanned")
	intelAddr := flag.String("intel", intel.DefaultAddr, "python intel service address")
	flag.Parse()

	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		log.Fatalf("root: %v", err)
	}

	s := &server{
		root:      rootAbs,
		webDir:    filepath.Join(rootAbs, "web"),
		intelAddr: *intelAddr,
	}
	s.allow = append(s.allow, rootAbs)
	if *allow != "" {
		for _, a := range strings.Split(*allow, ",") {
			if a = strings.TrimSpace(a); a == "" {
				continue
			}
			if abs, err := filepath.Abs(a); err == nil {
				s.allow = append(s.allow, abs)
			}
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/fixtures", s.handleFixtures)
	mux.HandleFunc("/api/scan", s.handleScan)
	mux.Handle("/", http.FileServer(http.Dir(s.webDir)))

	if !strings.HasPrefix(*addr, "127.0.0.1") && !strings.HasPrefix(*addr, "localhost") {
		log.Printf("WARNING: listening on %s — this exposes local source paths on the network", *addr)
	}
	log.Printf("PhantomDep %s — http://%s", version, *addr)
	log.Printf("root=%s analyser=%s", rootAbs, s.analyserLabel())
	if err := http.ListenAndServe(*addr, logRequests(mux)); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(started).Round(time.Millisecond))
		}
	})
}

func (s *server) analyserLabel() string {
	if bin, err := analyze.Locate(s.root); err == nil {
		return bin
	}
	return "not built (analysis stage will be skipped)"
}

// ------------------------------------------------------------------ routes --

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	stages := []model.Stage{}
	if _, err := analyze.Locate(s.root); err == nil {
		stages = append(stages, model.Stage{Name: "analyze", Status: model.StageOK, Detail: "rust analyser found"})
	} else {
		stages = append(stages, model.Stage{Name: "analyze", Status: model.StageSkipped, Detail: analyze.ErrNotFound.Error()})
	}
	if intel.Probe(s.intelAddr) {
		stages = append(stages, model.Stage{Name: "intel", Status: model.StageOK, Detail: s.intelAddr})
	} else {
		stages = append(stages, model.Stage{Name: "intel", Status: model.StageSkipped, Detail: "python service not running on " + s.intelAddr})
	}
	writeJSON(w, http.StatusOK, model.Health{OK: true, Version: version, Root: s.root, Stages: stages})
}

func (s *server) handleFixtures(w http.ResponseWriter, r *http.Request) {
	type fixture struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Note string `json:"note"`
	}
	dir := filepath.Join(s.root, "fixtures")
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"fixtures": []fixture{}})
		return
	}
	out := []fixture{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		full := filepath.Join(dir, e.Name())
		out = append(out, fixture{Name: e.Name(), Path: full, Note: firstLine(filepath.Join(full, "README.md"))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{"fixtures": out})
}

func (s *server) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use POST"})
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be {\"path\": \"...\"}"})
		return
	}
	res, code, err := s.scanPath(req.Path)
	if err != nil {
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ------------------------------------------------------------ orchestration --

func (s *server) scanPath(target string) (*model.Result, int, error) {
	if strings.TrimSpace(target) == "" {
		return nil, http.StatusBadRequest, errors.New("path is required")
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("cannot resolve %q: %w", target, err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	if abs == "/" || abs == os.Getenv("HOME") {
		return nil, http.StatusForbidden, fmt.Errorf("refusing to scan %s", abs)
	}
	if !s.allowed(abs) {
		return nil, http.StatusForbidden, fmt.Errorf("%s is outside the allowed roots (use -allow to widen)", abs)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return nil, http.StatusBadRequest, fmt.Errorf("%s is not a readable directory", abs)
	}

	started := time.Now()
	graph, findings, size, err := scan.Repository(abs)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("scan failed: %w", err)
	}

	stages := []model.Stage{{Name: "scan", Status: model.StageOK, MS: time.Since(started).Milliseconds()}}

	// --- stage: analyze (Rust) ---
	t0 := time.Now()
	var analysis model.Analysis
	if bin, err := analyze.Locate(s.root); err != nil {
		analysis = analyze.Fallback(graph)
		stages = append(stages, model.Stage{
			Name:   "analyze",
			Status: model.StageSkipped,
			Detail: "rust analyser not built — phantom detection only, no cycle analysis",
			MS:     time.Since(t0).Milliseconds(),
		})
	} else if a, runErr := analyze.Run(bin, graph); runErr != nil {
		analysis = analyze.Fallback(graph)
		stages = append(stages, model.Stage{
			Name:   "analyze",
			Status: model.StageDegraded,
			Detail: runErr.Error(),
			MS:     time.Since(t0).Milliseconds(),
		})
	} else {
		analysis = a
		stages = append(stages, model.Stage{Name: "analyze", Status: model.StageOK, MS: time.Since(t0).Milliseconds()})
	}

	// --- stage: intel (Python) ---
	t0 = time.Now()
	intelEntries := map[string]intel.Entry{}
	if !intel.Probe(s.intelAddr) {
		stages = append(stages, model.Stage{
			Name:   "intel",
			Status: model.StageSkipped,
			Detail: "python intel service not running on " + s.intelAddr,
			MS:     time.Since(t0).Milliseconds(),
		})
	} else if entries, ferr := intel.Fetch(s.intelAddr, graph.Packages); ferr != nil {
		stages = append(stages, model.Stage{
			Name:   "intel",
			Status: model.StageDegraded,
			Detail: ferr.Error(),
			MS:     time.Since(t0).Milliseconds(),
		})
	} else {
		intelEntries = entries
		stages = append(stages, model.Stage{Name: "intel", Status: model.StageOK, MS: time.Since(t0).Milliseconds()})
	}

	// --- merge ---
	for i := range graph.Packages {
		p := &graph.Packages[i]
		entry, ok := intelEntries[p.Ecosystem+":"+p.Name]
		if !ok {
			continue
		}
		p.License = entry.License
		p.Advisories = entry.Advisories
		if entry.Deprecated {
			findings = append(findings, model.Finding{
				Severity: "medium",
				Title:    "Deprecated package",
				Detail:   fmt.Sprintf("%s:%s is marked deprecated upstream", p.Ecosystem, p.Name),
				Package:  p.Name,
			})
		}
		for _, adv := range entry.Advisories {
			findings = append(findings, model.Finding{
				Severity: adv.Severity,
				Title:    "Advisory " + adv.ID,
				Detail:   adv.Summary,
				Package:  p.Name,
			})
		}
	}

	for _, idx := range analysis.Phantom {
		if idx < 0 || idx >= len(graph.Packages) {
			continue
		}
		p := graph.Packages[idx]
		findings = append(findings, model.Finding{
			Severity: "low",
			Title:    "Phantom dependency",
			Detail:   fmt.Sprintf("%s:%s is declared but never imported by any source file", p.Ecosystem, p.Name),
			Package:  p.Name,
		})
	}
	for _, idx := range analysis.Tombstones {
		if idx < 0 || idx >= len(graph.Packages) {
			continue
		}
		p := graph.Packages[idx]
		findings = append(findings, model.Finding{
			Severity: "medium",
			Title:    "Unpinned version",
			Detail:   fmt.Sprintf("%s:%s is declared with %q, which pins nothing", p.Ecosystem, p.Name, p.Version),
			Package:  p.Name,
		})
	}
	for _, cyc := range analysis.Cycles {
		names := make([]string, 0, len(cyc))
		for _, idx := range cyc {
			if idx >= 0 && idx < len(graph.Packages) {
				names = append(names, graph.Packages[idx].Name)
			}
		}
		findings = append(findings, model.Finding{
			Severity: "high",
			Title:    "Dependency cycle",
			Detail:   strings.Join(names, " -> ") + " -> " + firstOr(names, "?"),
		})
	}

	rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
	sort.SliceStable(findings, func(i, j int) bool {
		return rank[findings[i].Severity] < rank[findings[j].Severity]
	})

	res := &model.Result{
		Root:       graph.Root,
		ScannedAt:  time.Now().Format(time.RFC3339),
		Files:      size.Files,
		Lines:      size.Lines,
		Packages:   graph.Packages,
		Edges:      graph.Edges,
		Analysis:   analysis,
		Findings:   findings,
		Stages:     stages,
		DurationMS: time.Since(started).Milliseconds(),
	}
	res.Stats = buildStats(graph.Packages, analysis, findings)
	return res, http.StatusOK, nil
}

func buildStats(pkgs []model.Package, a model.Analysis, findings []model.Finding) model.Stats {
	st := model.Stats{Total: len(pkgs) - 1, Cycles: len(a.Cycles)}
	for _, p := range pkgs {
		if p.Ecosystem == "root" {
			continue
		}
		if p.Declared {
			st.Declared++
		}
		if p.Imported {
			st.Imported++
		}
		if p.Dev {
			st.Dev++
		}
		if p.State == model.StatePhantom {
			st.Phantom++
		}
		if p.State == model.StateUndeclared {
			st.Undeclared++
		}
		st.Advisories += len(p.Advisories)
	}
	return st
}

func (s *server) allowed(abs string) bool {
	for _, a := range s.allow {
		if abs == a {
			return true
		}
		if strings.HasPrefix(abs, a+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func firstLine(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if s := strings.TrimSpace(line); s != "" && !strings.HasPrefix(s, "#") {
			return s
		}
	}
	return ""
}

func firstOr(xs []string, fallback string) string {
	if len(xs) > 0 {
		return xs[0]
	}
	return fallback
}

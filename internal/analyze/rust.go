// Package analyze bridges the Go orchestrator to the Rust graph analyser.
//
// The bridge is a child process speaking a line protocol on stdin/stdout. It is
// deliberately not a CGo link: keeping the Rust stage out of process means a
// missing or crashing analyser degrades the scan instead of taking it down.
package analyze

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"phantomdep/internal/model"
)

// ProtocolVersion must match PROTOCOL_VERSION in analyzer-rs/src/main.rs.
const ProtocolVersion = "PHANTOMDEP-GRAPH 1"

// ErrNotFound is returned when no analyser binary can be located.
var ErrNotFound = errors.New("analyser binary not found; build it with scripts/build.sh")

// Locate finds the analyser binary, preferring an explicit override.
func Locate(root string) (string, error) {
	if env := os.Getenv("PHANTOMDEP_ANALYZER"); env != "" {
		if st, err := os.Stat(env); err == nil && !st.IsDir() {
			return env, nil
		}
	}
	candidates := []string{
		filepath.Join(root, "analyzer-rs", "target", "release", "phantomdep-analyzer"),
		filepath.Join(root, "analyzer-rs", "target", "debug", "phantomdep-analyzer"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", ErrNotFound
}

// Run executes the analyser and decodes its reply.
func Run(bin string, g *model.Graph) (model.Analysis, error) {
	var zero model.Analysis

	var in bytes.Buffer
	in.WriteString(ProtocolVersion + "\n")
	for i, p := range g.Packages {
		fmt.Fprintf(&in, "NODE %d declared=%d imported=%d dev=%d tomb=%d files=%d name=%s\n",
			i, b2i(p.Declared), b2i(p.Imported), b2i(p.Dev), b2i(tombstone(p)), p.Files, p.Name)
	}
	for _, e := range g.Edges {
		fmt.Fprintf(&in, "EDGE %d %d %s\n", e.From, e.To, e.Kind)
	}
	in.WriteString("DONE\n")

	cmd := exec.Command(bin)
	cmd.Stdin = &in
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return zero, fmt.Errorf("analyser failed: %s", msg)
	}

	var a model.Analysis
	if err := json.Unmarshal(stdout.Bytes(), &a); err != nil {
		return zero, fmt.Errorf("analyser returned unreadable JSON: %w", err)
	}
	return a, nil
}

// Fallback reproduces the parts of the analysis that do not need graph traversal.
// It exists so that a clone without a Rust toolchain still produces a usable
// report; the UI states which stages were skipped.
func Fallback(g *model.Graph) model.Analysis {
	a := model.Analysis{Phantom: []int{}, Undeclared: []int{}, Tombstones: []int{}, Cycles: [][]int{}}
	var phantomMass float64
	declaredCount := 0
	for i, p := range g.Packages {
		if i == 0 {
			continue // node 0 is the repository root, never a dependency
		}
		if p.Declared {
			declaredCount++
		}
		if p.Declared && !p.Imported {
			a.Phantom = append(a.Phantom, i)
			phantomMass += 1
		}
		if !p.Declared && p.Imported {
			a.Undeclared = append(a.Undeclared, i)
		}
		if p.Declared && tombstone(p) {
			a.Tombstones = append(a.Tombstones, i)
		}
	}
	a.FanIn = make([]int, len(g.Packages))
	for _, e := range g.Edges {
		if e.To >= 0 && e.To < len(a.FanIn) {
			a.FanIn[e.To]++
		}
	}
	if declaredCount > 0 {
		a.GhostScore = 100 * phantomMass / float64(declaredCount+1) * 0.6
	}
	a.MaxDepth = 1
	return a
}

func tombstone(p model.Package) bool {
	v := strings.TrimSpace(p.Version)
	if !p.Declared || p.Ecosystem == "root" {
		return false
	}
	switch v {
	case "", "*", "latest", "x", "X":
		return true
	}
	return false
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

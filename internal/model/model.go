// Package model holds the types that cross every stage boundary.
//
// Nothing in this package may import another internal package: it is the
// contract between the Go orchestrator, the Rust analyser and the Python
// intel service. Changing a field here is a protocol change.
package model

// Stage status values. Every pipeline stage reports one of these so the UI can
// render an honest banner instead of pretending a skipped stage succeeded.
const (
	StageOK       = "ok"
	StageSkipped  = "skipped"
	StageDegraded = "degraded"
)

// Advisory is a single vulnerability record for a package.
type Advisory struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	FixedIn  string `json:"fixedIn,omitempty"`
}

// Package states as rendered by the UI.
const (
	StateLive       = "live"       // declared and imported
	StatePhantom    = "phantom"    // declared, never imported
	StateUndeclared = "undeclared" // imported, never declared
	StateTransitive = "transitive" // only reachable through another package
	StateRoot       = "root"
)

// Package is one dependency node.
type Package struct {
	Name       string     `json:"name"`
	Ecosystem  string     `json:"ecosystem"`
	Version    string     `json:"version,omitempty"`
	Dev        bool       `json:"dev"`
	Declared   bool       `json:"declared"`
	Imported   bool       `json:"imported"`
	State      string     `json:"state"`
	License    string     `json:"license,omitempty"`
	Advisories []Advisory `json:"advisories,omitempty"`
	// Files counts how many source files reference this package. It is the
	// cheapest evidence a human can check when disputing a phantom verdict.
	Files int `json:"files,omitempty"`
}

// Edge kinds.
const (
	EdgeImport = "import" // root -> declared dependency
	EdgeDev    = "dev"    // root -> devDependency
	EdgeDep    = "dep"    // package -> package, read from a lockfile
)

// Edge is a directed dependency relation between two node indices.
type Edge struct {
	From int    `json:"from"`
	To   int    `json:"to"`
	Kind string `json:"kind"`
}

// Graph is the complete extraction output. Node 0 is always the repository root.
type Graph struct {
	Root     string
	Packages []Package
	Edges    []Edge
}

// Analysis is produced by the Rust stage. All indices refer to Graph.Packages.
type Analysis struct {
	Cycles     [][]int `json:"cycles"`
	MaxDepth   int     `json:"maxDepth"`
	GhostScore float64 `json:"ghostScore"`
	Phantom    []int   `json:"phantom"`
	Undeclared []int   `json:"undeclared"`
	Tombstones []int   `json:"tombstones"`
	FanIn      []int   `json:"fanIn"`
}

// Finding is a human-readable defect report.
type Finding struct {
	Severity string `json:"severity"` // critical | high | medium | low
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Package  string `json:"package,omitempty"`
	File     string `json:"file,omitempty"`
}

// Stage records the outcome of one pipeline stage.
type Stage struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	MS     int64  `json:"ms"`
}

// Stats are the headline numbers. Every value must be reproducible from the
// package list, so the UI can always show the raw counts behind the score.
type Stats struct {
	Total      int `json:"total"`
	Declared   int `json:"declared"`
	Imported   int `json:"imported"`
	Phantom    int `json:"phantom"`
	Undeclared int `json:"undeclared"`
	Dev        int `json:"dev"`
	Cycles     int `json:"cycles"`
	Advisories int `json:"advisories"`
}

// Result is the /api/scan response.
type Result struct {
	Root       string    `json:"root"`
	ScannedAt  string    `json:"scannedAt"`
	Files      int       `json:"files"`
	Lines      int       `json:"lines"`
	Packages   []Package `json:"packages"`
	Edges      []Edge    `json:"edges"`
	Analysis   Analysis  `json:"analysis"`
	Findings   []Finding `json:"findings"`
	Stages     []Stage   `json:"stages"`
	Stats      Stats     `json:"stats"`
	DurationMS int64     `json:"durationMs"`
}

// Health is the /api/health response.
type Health struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
	Root    string `json:"root"`
	Stages  []Stage `json:"stages"`
}

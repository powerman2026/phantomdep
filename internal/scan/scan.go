package scan

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"phantomdep/internal/model"
)

// Stats are the raw size measurements of a scan.
type Stats struct {
	Files int
	Lines int
}

// skipDirs is intentionally conservative: walking a node_modules or a Rust
// target directory would dominate the runtime and tell us nothing.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "target": true, "dist": true, "bin": true,
	".venv": true, "venv": true, "__pycache__": true, ".idea": true, ".vscode": true,
	"vendor": true, ".next": true, "coverage": true, ".cache": true,
}

var sourceExts = map[string]bool{
	".js": true, ".mjs": true, ".cjs": true, ".jsx": true,
	".ts": true, ".tsx": true, ".vue": true,
	".go": true, ".py": true, ".rs": true,
}

type collected struct {
	manifests []string
	sources   []string
}

// Repository walks abs and returns the dependency graph (R1.1, R2, R3).
func Repository(abs string) (*model.Graph, []model.Finding, Stats, error) {
	var stats Stats
	var findings []model.Finding

	files, err := collect(abs, &stats)
	if err != nil {
		return nil, nil, stats, err
	}

	declared := map[string]declaredPkg{}
	var lockEdges []lockEdge
	// Lockfiles also introduce packages that no manifest mentions. They are real
	// nodes in the graph (state "transitive") and are what makes depth and
	// cycle analysis meaningful on a real repository.
	lockNodes := map[string]bool{}

	for _, rel := range files.manifests {
		full := filepath.Join(abs, rel)
		raw, err := readFile(full)
		if err != nil {
			findings = append(findings, model.Finding{
				Severity: "medium",
				Title:    "Unreadable manifest",
				Detail:   err.Error(),
				File:     rel,
			})
			continue
		}
		var res manifestResult
		switch filepath.Base(rel) {
		case "package.json":
			res = ParsePackageJSON(rel, raw)
		case "package-lock.json":
			res = ParsePackageLock(rel, raw)
		case "go.mod":
			res = ParseGoMod(rel, raw)
		case "requirements.txt":
			res = ParseRequirementsTxt(rel, raw)
		case "Cargo.toml":
			res = ParseCargoToml(rel, raw)
		}
		findings = append(findings, res.Findings...)
		for _, p := range res.Pkgs {
			key := p.Ecosystem + ":" + p.Name
			if prev, ok := declared[key]; ok {
				// A dependency listed as both prod and dev counts as prod.
				prev.Dev = prev.Dev && p.Dev
				if prev.Version == "" {
					prev.Version = p.Version
				}
				declared[key] = prev
				continue
			}
			declared[key] = p
		}
		lockEdges = append(lockEdges, res.Lock...)
		for _, le := range res.Lock {
			lockNodes[le.From] = true
			lockNodes[le.To] = true
		}
	}

	imports := map[string]int{} // key -> file count
	for _, rel := range files.sources {
		full := filepath.Join(abs, rel)
		raw, err := readFile(full)
		if err != nil {
			continue
		}
		seen := map[string]bool{}
		for _, ref := range ExtractImports(rel, raw) {
			key := ref.Ecosystem + ":" + ref.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			imports[key]++
		}
	}

	// Union of declared, imported and lockfile-only packages.
	keys := map[string]bool{}
	for k := range declared {
		keys[k] = true
	}
	for k := range imports {
		keys[k] = true
	}
	for k := range lockNodes {
		keys[k] = true
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Slice(ordered, func(i, j int) bool {
		ei, ni := splitKey(ordered[i])
		ej, nj := splitKey(ordered[j])
		if ei != ej {
			return ei < ej
		}
		return ni < nj
	})

	rootName := filepath.Base(abs)
	root := model.Package{
		Name:      rootName,
		Ecosystem: EcoRoot,
		Declared:  true,
		Imported:  true,
		State:     model.StateRoot,
	}

	packages := make([]model.Package, 0, len(ordered)+1)
	packages = append(packages, root)
	index := map[string]int{}

	for _, key := range ordered {
		eco, name := splitKey(key)
		d, isDeclared := declared[key]
		nFiles := imports[key]
		isImported := nFiles > 0

		p := model.Package{
			Name:      name,
			Ecosystem: eco,
			Version:   d.Version,
			Dev:       d.Dev,
			Declared:  isDeclared,
			Imported:  isImported,
			Files:     nFiles,
		}
		switch {
		case isDeclared && isImported:
			p.State = model.StateLive
		case isDeclared && !isImported:
			p.State = model.StatePhantom
		case !isDeclared && isImported:
			p.State = model.StateUndeclared
		default:
			p.State = model.StateTransitive
		}
		index[key] = len(packages)
		packages = append(packages, p)
	}

	// root -> declared edges
	var edges []model.Edge
	for _, key := range ordered {
		d, ok := declared[key]
		if !ok {
			continue
		}
		kind := model.EdgeImport
		if d.Dev {
			kind = model.EdgeDev
		}
		edges = append(edges, model.Edge{From: 0, To: index[key], Kind: kind})
	}

	// package -> package edges from lockfiles
	seenEdge := map[[2]int]bool{}
	for _, le := range lockEdges {
		from, okF := index[le.From]
		to, okT := index[le.To]
		if !okF || !okT || from == to {
			continue
		}
		k := [2]int{from, to}
		if seenEdge[k] {
			continue
		}
		seenEdge[k] = true
		edges = append(edges, model.Edge{From: from, To: to, Kind: model.EdgeDep})
	}

	// Findings for undeclared imports: the highest-signal defect class.
	for _, key := range ordered {
		p := packages[index[key]]
		if p.State == model.StateUndeclared {
			findings = append(findings, model.Finding{
				Severity: "high",
				Title:    "Undeclared dependency",
				Detail:   fmt.Sprintf("%s:%s is imported in %d file(s) but absent from every manifest", p.Ecosystem, p.Name, p.Files),
				Package:  p.Name,
			})
		}
	}

	return &model.Graph{Root: rootName, Packages: packages, Edges: edges}, findings, stats, nil
}

func splitKey(key string) (string, string) {
	idx := strings.Index(key, ":")
	if idx < 0 {
		return "", key
	}
	return key[:idx], key[idx+1:]
}

// collect walks the tree once, recording manifest and source paths (R1.1).
func collect(abs string, stats *Stats) (collected, error) {
	var out collected
	err := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip, do not abort the scan
		}
		rel, relErr := filepath.Rel(abs, p)
		if relErr != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		base := d.Name()
		switch base {
		case "package.json", "package-lock.json", "go.mod", "requirements.txt", "Cargo.toml":
			out.manifests = append(out.manifests, rel)
			return nil
		}
		if sourceExts[strings.ToLower(filepath.Ext(base))] {
			out.sources = append(out.sources, rel)
			stats.Files++
			if info, statErr := d.Info(); statErr == nil && info.Size() < 2<<20 {
				if raw, readErr := os.ReadFile(p); readErr == nil {
					stats.Lines += strings.Count(string(raw), "\n")
				}
			}
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	sort.Strings(out.manifests)
	sort.Strings(out.sources)
	return out, nil
}

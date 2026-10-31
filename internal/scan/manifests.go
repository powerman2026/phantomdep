package scan

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"phantomdep/internal/model"
)

// Ecosystem identifiers used throughout the pipeline.
const (
	EcoNPM   = "npm"
	EcoGo    = "go"
	EcoPyPI  = "pypi"
	EcoCargo = "cargo"
	EcoRoot  = "root"
)

// declaredPkg is an intermediate record produced by a manifest parser.
type declaredPkg struct {
	Name      string
	Ecosystem string
	Version   string
	Dev       bool
}

// lockEdge is a package -> package relation read from a lockfile.
type lockEdge struct {
	From string // normalised "ecosystem:name"
	To   string
}

// manifestResult is what every parser returns.
type manifestResult struct {
	Pkgs     []declaredPkg
	Lock     []lockEdge
	Findings []model.Finding
}

// ParsePackageJSON handles package.json (R2.1).
func ParsePackageJSON(rel string, raw []byte) manifestResult {
	var doc struct {
		Name            string            `json:"name"`
		Version         string            `json:"version"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	var out manifestResult
	if err := json.Unmarshal(raw, &doc); err != nil {
		out.Findings = append(out.Findings, model.Finding{
			Severity: "high",
			Title:    "Malformed package.json",
			Detail:   fmt.Sprintf("%s could not be parsed: %v", rel, err),
			File:     rel,
		})
		return out
	}
	for name, ver := range doc.Dependencies {
		out.Pkgs = append(out.Pkgs, declaredPkg{Name: name, Ecosystem: EcoNPM, Version: ver})
	}
	for name, ver := range doc.DevDependencies {
		out.Pkgs = append(out.Pkgs, declaredPkg{Name: name, Ecosystem: EcoNPM, Version: ver, Dev: true})
	}
	return out
}

// ParsePackageLock reads package-lock.json v2/v3 and extracts package -> package
// edges. This is the only source of real inter-package edges for npm, which is
// what makes cycle detection meaningful rather than decorative.
func ParsePackageLock(rel string, raw []byte) manifestResult {
	var doc struct {
		Name     string `json:"name"`
		Packages map[string]struct {
			Version      string            `json:"version"`
			Dev          bool              `json:"dev"`
			Dependencies map[string]string `json:"dependencies"`
		} `json:"packages"`
	}
	var out manifestResult
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Packages) == 0 {
		return out
	}

	keys := make([]string, 0, len(doc.Packages))
	for k := range doc.Packages {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// npm resolves a dependency by walking up the node_modules chain, so a
	// nested copy wins over the hoisted one.
	resolve := func(from, dep string) (string, bool) {
		if from != "" {
			nested := from + "/node_modules/" + dep
			if _, ok := doc.Packages[nested]; ok {
				return nested, true
			}
		}
		hoisted := "node_modules/" + dep
		if _, ok := doc.Packages[hoisted]; ok {
			return hoisted, true
		}
		return "", false
	}

	norm := func(key string) string {
		name := strings.TrimPrefix(key, "node_modules/")
		// keep the last segment group so "@scope/pkg" survives
		if strings.Contains(name, "node_modules/") {
			idx := strings.LastIndex(name, "node_modules/")
			name = name[idx+len("node_modules/"):]
		}
		return EcoNPM + ":" + name
	}

	for _, from := range keys {
		if from == "" {
			continue // root entry: its deps are the manifest's job
		}
		for dep := range doc.Packages[from].Dependencies {
			if target, ok := resolve(from, dep); ok {
				out.Lock = append(out.Lock, lockEdge{From: norm(from), To: norm(target)})
			}
		}
	}
	return out
}

// ParseGoMod handles go.mod (R2.2), including block require form.
func ParseGoMod(rel string, raw []byte) manifestResult {
	var out manifestResult
	lines := strings.Split(string(raw), "\n")
	inBlock := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "require ("):
			inBlock = true
			continue
		case inBlock && trimmed == ")":
			inBlock = false
			continue
		case strings.HasPrefix(trimmed, "module "):
			continue
		case strings.HasPrefix(trimmed, "require "):
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "require "))
		case !inBlock:
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if !strings.Contains(name, ".") {
			out.Findings = append(out.Findings, model.Finding{
				Severity: "medium",
				Title:    "Suspicious go.mod require entry",
				Detail:   fmt.Sprintf("line %d of %s: %q has no domain component", i+1, rel, name),
				File:     rel,
			})
			continue
		}
		ver := ""
		if len(fields) > 1 {
			ver = fields[1]
		}
		out.Pkgs = append(out.Pkgs, declaredPkg{Name: name, Ecosystem: EcoGo, Version: ver})
	}
	return out
}

// ParseRequirementsTxt handles requirements.txt (R2.3).
func ParseRequirementsTxt(rel string, raw []byte) manifestResult {
	var out manifestResult
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "-") {
			continue // -r, -e, --index-url and friends are not packages
		}
		if idx := strings.Index(line, "#"); idx > 0 {
			line = strings.TrimSpace(line[:idx])
		}
		// drop environment markers and extras
		if idx := strings.IndexAny(line, ";["); idx > 0 {
			line = strings.TrimSpace(line[:idx])
		}
		name := line
		ver := ""
		if idx := strings.IndexAny(line, "=<>!~"); idx > 0 {
			name = strings.TrimSpace(line[:idx])
			ver = strings.TrimSpace(line[idx:])
		}
		if name == "" {
			continue
		}
		out.Pkgs = append(out.Pkgs, declaredPkg{Name: strings.ToLower(name), Ecosystem: EcoPyPI, Version: ver})
	}
	return out
}

// ParseCargoToml handles the [dependencies] and [dev-dependencies] tables
// (R2.4). Workspace inheritance entries (`foo.workspace = true`) are recorded
// with an empty version rather than guessed.
func ParseCargoToml(rel string, raw []byte) manifestResult {
	var out manifestResult
	section := ""
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.Trim(trimmed, "[]")
			continue
		}
		if section != "dependencies" && section != "dev-dependencies" {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		if strings.Contains(key, ".") {
			continue // foo.workspace / foo.version sub-key
		}
		val := strings.TrimSpace(trimmed[eq+1:])
		ver := ""
		if strings.HasPrefix(val, "\"") {
			ver = strings.Trim(val, "\"")
		} else if strings.Contains(val, "version") {
			if i := strings.Index(val, "version"); i >= 0 {
				rest := val[i:]
				if q := strings.Index(rest, "\""); q >= 0 {
					if q2 := strings.Index(rest[q+1:], "\""); q2 >= 0 {
						ver = rest[q+1 : q+1+q2]
					}
				}
			}
		}
		out.Pkgs = append(out.Pkgs, declaredPkg{
			Name:      key,
			Ecosystem: EcoCargo,
			Version:   ver,
			Dev:       section == "dev-dependencies",
		})
	}
	return out
}

// readFile limits reads so a stray multi-gigabyte artefact cannot stall a scan.
func readFile(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > 4<<20 {
		return nil, fmt.Errorf("%s is larger than 4 MiB", path)
	}
	return os.ReadFile(path)
}

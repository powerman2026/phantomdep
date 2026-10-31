package scan

import (
	"path/filepath"
	"regexp"
	"strings"
)

// ImportRef is one module reference found in a source file.
type ImportRef struct {
	Ecosystem string
	Name      string
	File      string
}

var (
	reJSFrom    = regexp.MustCompile(`(?m)(?:^|\s)(?:import|export)\s[^;'"]*?from\s*['"]([^'"]+)['"]`)
	reJSRequire = regexp.MustCompile(`(?m)require\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	reJSDynamic = regexp.MustCompile(`(?m)import\s*\(\s*['"]([^'"]+)['"]\s*\)`)

	rePyFrom   = regexp.MustCompile(`(?m)^\s*from\s+([A-Za-z_][A-Za-z0-9_.]*)\s+import\b`)
	rePyImport = regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z_][A-Za-z0-9_.]*(?:\s*,\s*[A-Za-z_][A-Za-z0-9_.]*)*)`)

	reRustUse = regexp.MustCompile(`(?m)^\s*use\s+([A-Za-z_][A-Za-z0-9_]*)`)

	reGoImportBlock = regexp.MustCompile(`(?s)import\s*\(([^)]*)\)`)
	reGoImportOne   = regexp.MustCompile(`(?m)^\s*import\s+(?:[A-Za-z_.]\w*\s+)?"([^"]+)"`)
	reGoQuoted      = regexp.MustCompile(`"([^"]+)"`)
)

// pythonStdlib is a conservative allowlist. Anything not listed and not
// importable from the repository is reported as undeclared, so a missing entry
// produces a false positive rather than a silent false negative.
var pythonStdlib = map[string]bool{
	"__future__": true,
	"abc": true, "argparse": true, "array": true, "asyncio": true, "base64": true,
	"bisect": true, "builtins": true, "calendar": true, "codecs": true, "collections": true,
	"concurrent": true, "contextlib": true, "copy": true, "csv": true, "dataclasses": true,
	"datetime": true, "decimal": true, "difflib": true, "email": true, "enum": true,
	"fractions": true, "functools": true, "getpass": true, "glob": true, "gzip": true,
	"hashlib": true, "heapq": true, "html": true, "http": true, "importlib": true,
	"inspect": true, "io": true, "itertools": true, "json": true, "locale": true,
	"logging": true, "math": true, "mmap": true, "multiprocessing": true, "numbers": true,
	"operator": true, "os": true, "pathlib": true, "pickle": true, "platform": true,
	"pprint": true, "queue": true, "random": true, "re": true, "secrets": true,
	"select": true, "shutil": true, "signal": true, "socket": true, "sqlite3": true,
	"ssl": true, "stat": true, "statistics": true, "string": true, "struct": true,
	"subprocess": true, "sys": true, "tarfile": true, "tempfile": true, "textwrap": true,
	"threading": true, "time": true, "traceback": true, "types": true, "typing": true,
	"unittest": true, "urllib": true, "uuid": true, "warnings": true, "weakref": true,
	"xml": true, "zipfile": true, "zlib": true,
}

// rustBuiltin holds crate roots that never appear in [dependencies].
var rustBuiltin = map[string]bool{
	"std": true, "core": true, "alloc": true, "crate": true, "self": true,
	"super": true, "proc_macro": true, "test": true,
}

// ExtractImports pulls module references out of one source file (R3.1-R3.5).
func ExtractImports(rel string, content []byte) []ImportRef {
	ext := strings.ToLower(filepath.Ext(rel))
	src := string(content)
	var out []ImportRef

	add := func(eco, name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		out = append(out, ImportRef{Ecosystem: eco, Name: name, File: rel})
	}

	switch ext {
	case ".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx", ".vue":
		for _, re := range []*regexp.Regexp{reJSFrom, reJSRequire, reJSDynamic} {
			for _, m := range re.FindAllStringSubmatch(src, -1) {
				if name, ok := normaliseNPM(m[1]); ok {
					add(EcoNPM, name)
				}
			}
		}
	case ".go":
		for _, block := range reGoImportBlock.FindAllStringSubmatch(src, -1) {
			for _, q := range reGoQuoted.FindAllStringSubmatch(block[1], -1) {
				if name, ok := normaliseGo(q[1]); ok {
					add(EcoGo, name)
				}
			}
		}
		for _, m := range reGoImportOne.FindAllStringSubmatch(src, -1) {
			if name, ok := normaliseGo(m[1]); ok {
				add(EcoGo, name)
			}
		}
	case ".py":
		for _, m := range rePyFrom.FindAllStringSubmatch(src, -1) {
			if name, ok := normalisePy(m[1]); ok {
				add(EcoPyPI, name)
			}
		}
		for _, m := range rePyImport.FindAllStringSubmatch(src, -1) {
			for _, part := range strings.Split(m[1], ",") {
				if name, ok := normalisePy(part); ok {
					add(EcoPyPI, name)
				}
			}
		}
	case ".rs":
		for _, m := range reRustUse.FindAllStringSubmatch(src, -1) {
			name := strings.ReplaceAll(m[1], "_", "-")
			if rustBuiltin[m[1]] {
				continue
			}
			add(EcoCargo, name)
		}
	}
	return out
}

// normaliseNPM strips relative paths and sub-paths (R3.5).
func normaliseNPM(spec string) (string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") {
		return "", false
	}
	if strings.HasPrefix(spec, "node:") {
		return "", false
	}
	if strings.HasPrefix(spec, "@") {
		parts := strings.Split(spec, "/")
		if len(parts) < 2 {
			return "", false
		}
		return parts[0] + "/" + parts[1], true
	}
	return strings.Split(spec, "/")[0], true
}

// normaliseGo keeps only third-party paths (R3.2).
func normaliseGo(spec string) (string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}
	first := strings.Split(spec, "/")[0]
	if !strings.Contains(first, ".") {
		return "", false // standard library
	}
	return spec, true
}

// normalisePy reduces a dotted module path to its root (R3.5).
func normalisePy(spec string) (string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}
	root := strings.Split(spec, ".")[0]
	if root == "" || pythonStdlib[root] {
		return "", false
	}
	return strings.ToLower(root), true
}

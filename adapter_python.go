package coupling

import (
	"path/filepath"
	"strings"
)

// pythonAdapter computes package-level coupling for a Python project. Nodes are
// packages — the dotted path of a file's directory beneath the import root (a
// top-level src/ when present, else the project root). Absolute imports resolve
// to the longest first-party package prefix; relative imports (leading dots)
// are resolved against the importing file's package.
type pythonAdapter struct {
	importRoot string
	module     string
}

func (a *pythonAdapter) matchesLanguage(lang string) bool { return lang == "python" }

func (a *pythonAdapter) prepare(root string) (string, bool) {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	a.importRoot = abs
	if src := filepath.Join(abs, "src"); dirExists(src) {
		a.importRoot = src // src-layout
	}
	a.module = filepath.Base(abs)
	return a.module, true
}

func (a *pythonAdapter) node(f File) string {
	abs := f.Path
	if !filepath.IsAbs(abs) {
		if p, err := filepath.Abs(abs); err == nil {
			abs = p
		}
	}
	rel, err := filepath.Rel(a.importRoot, filepath.Dir(abs))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "" // outside the import root
	}
	if rel == "." || rel == "" {
		return "" // top-level module — no package
	}
	return strings.ReplaceAll(filepath.ToSlash(rel), "/", ".")
}

func (a *pythonAdapter) firstPartyImport(imp, fromNode string, nodes map[string]bool) (string, bool) {
	if strings.HasPrefix(imp, ".") {
		fq, ok := resolveRelativePythonImport(imp, fromNode)
		if !ok {
			return "", false
		}
		return longestPackagePrefix(fq, nodes, ".")
	}
	return longestPackagePrefix(imp, nodes, ".")
}

// resolveRelativePythonImport turns a dotted relative import into a fully
// qualified module path anchored at the importing file's package (fromNode).
// One leading dot is the current package; each extra dot ascends one level.
func resolveRelativePythonImport(imp, fromNode string) (string, bool) {
	dots := 0
	for dots < len(imp) && imp[dots] == '.' {
		dots++
	}
	remainder := imp[dots:]

	var segs []string
	if fromNode != "" {
		segs = strings.Split(fromNode, ".")
	}
	up := dots - 1
	if up > len(segs) {
		return "", false // climbs above the import root
	}
	segs = segs[:len(segs)-up]
	if remainder != "" {
		segs = append(segs, remainder)
	}
	if len(segs) == 0 {
		return "", false
	}
	return strings.Join(segs, "."), true
}

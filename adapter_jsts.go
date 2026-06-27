package coupling

import (
	"path/filepath"
	"strings"
)

// jstsAdapter computes directory-module coupling for a JavaScript / TypeScript
// project. JS/TS has no package declaration, so the coupling unit is the
// directory: node = a file's directory relative to the project root. First-party
// is structural — a relative import (./x, ../y) is first-party; a bare specifier
// (react, @scope/pkg) is external. It spans both languages.
type jstsAdapter struct {
	root   string
	module string
}

func (a *jstsAdapter) matchesLanguage(lang string) bool {
	return lang == "javascript" || lang == "typescript"
}

func (a *jstsAdapter) prepare(root string) (string, bool) {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	a.root = abs
	a.module = filepath.Base(abs)
	return a.module, true
}

func (a *jstsAdapter) node(f File) string {
	abs := absolutise(f.Path)
	rel, err := filepath.Rel(a.root, filepath.Dir(abs))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel) // "." for the root directory
}

func (a *jstsAdapter) firstPartyImport(imp, fromNode string, nodes map[string]bool) (string, bool) {
	imp = strings.TrimSpace(imp)
	if !strings.HasPrefix(imp, ".") {
		return "", false // bare specifier / alias → external
	}
	base := fromNode
	if base == "." {
		base = ""
	}
	resolved := filepath.Clean(filepath.Join(filepath.FromSlash(base), filepath.FromSlash(imp)))
	resolvedSlash := filepath.ToSlash(resolved)
	if resolvedSlash == ".." || strings.HasPrefix(resolvedSlash, "../") {
		return "", false // climbs outside the project
	}
	// Directory import (./sub) vs file import (./sib): a directory module iff
	// it's in the node set (it holds files), avoiding an os.Stat per import.
	if nodes[resolvedSlash] {
		return resolvedSlash, true
	}
	targetDir := filepath.ToSlash(filepath.Dir(resolved))
	if targetDir == "" {
		targetDir = "."
	}
	return targetDir, true
}

// absolutise resolves a possibly-relative path against the process cwd.
func absolutise(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if p, err := filepath.Abs(path); err == nil {
		return p
	}
	return path
}

package coupling

import (
	"path/filepath"
	"strings"
)

// goAdapter resolves Go packages: the first-party boundary is the go.mod module
// path and a package node is module + the file's directory.
type goAdapter struct {
	root   string
	module string
}

func (a *goAdapter) matchesLanguage(lang string) bool { return lang == "go" }

func (a *goAdapter) prepare(root string) (string, bool) {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	a.root = abs
	a.module = moduledPath(root)
	return a.module, a.module != ""
}

func (a *goAdapter) node(f File) string {
	path := f.Path
	if !filepath.IsAbs(path) {
		if p, err := filepath.Abs(path); err == nil {
			path = p
		}
	}
	return goPackageImportPath(a.root, path, a.module)
}

func (a *goAdapter) firstPartyImport(imp, _ string, _ map[string]bool) (string, bool) {
	if isFirstParty(imp, a.module) {
		return imp, true // a Go import string IS the package path
	}
	return "", false
}

// goPackageImportPath maps a Go file's disk path to its package import path
// (module + the file's directory relative to root). Files directly in the
// module root resolve to the module path. "" when the file sits outside root.
func goPackageImportPath(root, path, module string) string {
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" {
		return module
	}
	return module + "/" + rel
}

// isFirstParty reports whether an import path belongs to the module.
func isFirstParty(imp, module string) bool {
	return imp == module || strings.HasPrefix(imp, module+"/")
}

package coupling

import (
	"path/filepath"
	"strings"
)

// rubyAdapter computes directory-module coupling for a Ruby gem. Like JS/TS,
// the coupling unit is the directory. Because require / require_relative target
// a FILE (not a directory) and arrive without a leading dot to distinguish load-
// path from file-relative, the adapter records every first-party file's load-
// path stem during the node pass and resolves an import by stem lookup — so an
// import is first-party only when a real first-party file backs it (no stdlib /
// gem false positives).
type rubyAdapter struct {
	root     string
	module   string
	loadRoot string            // "lib" when <root>/lib exists, else ""
	stems    map[string]string // load-path stem -> the file's directory node
}

func (a *rubyAdapter) matchesLanguage(lang string) bool { return lang == "ruby" }

func (a *rubyAdapter) prepare(root string) (string, bool) {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	a.root = abs
	a.module = filepath.Base(abs)
	a.stems = map[string]string{}
	if dirExists(filepath.Join(abs, "lib")) {
		a.loadRoot = "lib"
	}
	return a.module, true
}

// node returns a file's directory relative to root and records its load-path
// stem so firstPartyImport can resolve require / require_relative.
func (a *rubyAdapter) node(f File) string {
	abs := absolutise(f.Path)
	relFile, err := filepath.Rel(a.root, abs)
	if err != nil || relFile == ".." || strings.HasPrefix(relFile, ".."+string(filepath.Separator)) {
		return ""
	}
	relSlash := filepath.ToSlash(relFile)
	dirNode := filepath.ToSlash(filepath.Dir(relFile)) // "." for a root-level file
	stem := strings.TrimSuffix(relSlash, ".rb")
	if a.loadRoot != "" {
		stem = strings.TrimPrefix(stem, a.loadRoot+"/")
	}
	a.stems[stem] = dirNode
	return dirNode
}

func (a *rubyAdapter) firstPartyImport(imp, fromNode string, _ map[string]bool) (string, bool) {
	imp = strings.TrimSuffix(strings.TrimSpace(imp), ".rb")
	// (a) load-path require: the import string IS the stem.
	if dn, ok := a.stems[filepath.ToSlash(imp)]; ok {
		return dn, true
	}
	// (b) require_relative: resolve against the requiring file's directory.
	base := fromNode
	if base == "." {
		base = ""
	}
	relTarget := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.FromSlash(base), filepath.FromSlash(imp))))
	if relTarget == ".." || strings.HasPrefix(relTarget, "../") {
		return "", false
	}
	stem := relTarget
	if a.loadRoot != "" {
		stem = strings.TrimPrefix(stem, a.loadRoot+"/")
	}
	if dn, ok := a.stems[stem]; ok {
		return dn, true
	}
	return "", false
}

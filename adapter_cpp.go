package coupling

import (
	"path/filepath"
	"strings"
)

// cppAdapter computes directory-module coupling for a C / C++ project. C/C++ has
// no language-level module system, so the dependency graph IS the #include
// graph and the coupling unit is the directory: node = a file's directory
// relative to the project root. One adapter spans both "c" and "cpp" (a .h
// header detects as "c" even in a C++ project).
//
// The captured include text is bare (quotes / angle brackets stripped), so
// `#include "format.h"` and `#include <algorithm>` are indistinguishable by
// syntax. As in the Ruby adapter, an include is first-party only when it
// resolves to a real first-party FILE: the adapter records every file's path
// during the node pass, then resolves an include against the includer's
// directory and the common include roots (include/, src/, the project root).
// A system header backs no first-party file, so it's never an edge.
type cppAdapter struct {
	root         string
	module       string
	includeRoots []string          // "" (root) + "include" / "src" when present
	files        map[string]string // rel file path (slash) -> the file's directory node
}

func (a *cppAdapter) matchesLanguage(lang string) bool {
	return lang == "c" || lang == "cpp"
}

func (a *cppAdapter) prepare(root string) (string, bool) {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	a.root = abs
	a.module = filepath.Base(abs)
	a.files = map[string]string{}
	a.includeRoots = []string{""}
	for _, r := range []string{"include", "src"} {
		if dirExists(filepath.Join(abs, r)) {
			a.includeRoots = append(a.includeRoots, r)
		}
	}
	return a.module, true
}

// node returns a file's directory relative to root and records the file's rel
// path so firstPartyImport can tell a first-party header from a system one.
func (a *cppAdapter) node(f File) string {
	abs := absolutise(f.Path)
	relFile, err := filepath.Rel(a.root, abs)
	if err != nil || relFile == ".." || strings.HasPrefix(relFile, ".."+string(filepath.Separator)) {
		return ""
	}
	dirNode := filepath.ToSlash(filepath.Dir(relFile))
	a.files[filepath.ToSlash(relFile)] = dirNode
	return dirNode
}

// firstPartyImport resolves an #include to the directory of the first-party
// file it targets, or ok=false for a system / external header. Tries the
// includer's own directory first, then each include root.
func (a *cppAdapter) firstPartyImport(imp, fromNode string, _ map[string]bool) (string, bool) {
	imp = strings.TrimSpace(imp)
	if imp == "" {
		return "", false
	}
	base := fromNode
	if base == "." {
		base = ""
	}
	try := func(prefix string) (string, bool) {
		resolved := filepath.ToSlash(filepath.Join(prefix, imp))
		if resolved == ".." || strings.HasPrefix(resolved, "../") {
			return "", false
		}
		dn, ok := a.files[resolved]
		return dn, ok
	}
	if dn, ok := try(base); ok {
		return dn, true
	}
	for _, r := range a.includeRoots {
		if r == base {
			continue
		}
		if dn, ok := try(r); ok {
			return dn, true
		}
	}
	return "", false
}

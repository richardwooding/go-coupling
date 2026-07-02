package coupling

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// swiftAdapter computes module-level coupling for a SwiftPM package. A Swift
// module is a build target, not a directory or an in-source declaration, so the
// first-party boundary is the set of target names declared in Package.swift plus
// the conventional Sources/<Target>/ and Tests/<Target>/ layout. `import Foo`
// resolves to a first-party node iff Foo names one of those targets; Foundation
// and external SwiftPM package modules are external. `import Foo.Sub` takes the
// leading module segment (Foo).
//
// Package.swift is Swift code, not declarative data, so a faithful resolution
// needs the Swift toolchain. This is a pragmatic parse: it reads target names
// (and any explicit path:) from the common .target / .executableTarget /
// .testTarget / .macro / .plugin / .systemLibrary forms, and otherwise falls
// back to the conventional Sources/<Target>/ layout. Exotic manifests (computed
// target lists, custom sources: arrays) may resolve imperfectly.
type swiftAdapter struct {
	modules map[string]bool
	dirs    []swiftModuleDir
}

type swiftModuleDir struct {
	dir    string // absolute, cleaned target source directory
	module string
}

func (a *swiftAdapter) matchesLanguage(lang string) bool { return lang == "swift" }

// swiftTargetRe captures the name: argument of a SwiftPM target declaration.
var swiftTargetRe = regexp.MustCompile(`\.(?:testTarget|executableTarget|target|macro|systemLibrary|plugin)\s*\(\s*name:\s*"([^"]+)"`)

// swiftPathRe captures an explicit path: within a target declaration.
var swiftPathRe = regexp.MustCompile(`path:\s*"([^"]+)"`)

func (a *swiftAdapter) prepare(root string) (string, bool) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	a.modules = map[string]bool{}
	a.dirs = nil
	seen := map[string]bool{} // dedupe by dir path

	addDir := func(module, dir string) {
		dir = filepath.Clean(dir)
		if module == "" || seen[dir] {
			return
		}
		seen[dir] = true
		a.dirs = append(a.dirs, swiftModuleDir{dir: dir, module: module})
	}

	// 1. Target names (and explicit paths) declared in Package.swift.
	for name, p := range parseSwiftTargets(filepath.Join(absRoot, "Package.swift")) {
		a.modules[name] = true
		if p != "" {
			addDir(name, filepath.Join(absRoot, filepath.FromSlash(p)))
			continue
		}
		for _, base := range []string{"Sources", "Tests", "src"} {
			if cand := filepath.Join(absRoot, base, name); dirExists(cand) {
				addDir(name, cand)
				break
			}
		}
	}

	// 2. Convention: each immediate subdir of Sources/ and Tests/ is a module
	//    named after the subdir. Supplements (and, for manifest-less discovery,
	//    replaces) the declared set.
	for _, base := range []string{"Sources", "Tests"} {
		entries, rderr := os.ReadDir(filepath.Join(absRoot, base))
		if rderr != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			a.modules[e.Name()] = true
			addDir(e.Name(), filepath.Join(absRoot, base, e.Name()))
		}
	}

	if len(a.modules) == 0 {
		return "", false
	}
	// Longest directory first so node() resolves the most specific target.
	sort.Slice(a.dirs, func(i, j int) bool { return len(a.dirs[i].dir) > len(a.dirs[j].dir) })
	return filepath.Base(absRoot), true
}

func (a *swiftAdapter) node(f File) string {
	abs := f.Path
	if !filepath.IsAbs(abs) {
		if p, err := filepath.Abs(abs); err == nil {
			abs = p
		}
	}
	for _, md := range a.dirs {
		if abs == md.dir || strings.HasPrefix(abs, md.dir+string(filepath.Separator)) {
			return md.module
		}
	}
	return ""
}

func (a *swiftAdapter) firstPartyImport(imp, fromNode string, _ map[string]bool) (string, bool) {
	leading := imp
	if before, _, ok := strings.Cut(imp, "."); ok {
		leading = before // import Foo.Sub → module Foo
	}
	leading = strings.TrimSpace(leading)
	if leading == fromNode {
		return fromNode, true // intra-module reference — no inter-module edge
	}
	if a.modules[leading] {
		return leading, true
	}
	return "", false
}

// parseSwiftTargets returns declared SwiftPM target names mapped to their
// explicit path: (empty when the conventional layout applies), or nil when the
// manifest is unreadable.
func parseSwiftTargets(manifestPath string) map[string]string {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}
	src := string(data)
	locs := swiftTargetRe.FindAllStringSubmatchIndex(src, -1)
	out := make(map[string]string, len(locs))
	for i, loc := range locs {
		name := src[loc[2]:loc[3]]
		end := len(src)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		path := ""
		if m := swiftPathRe.FindStringSubmatch(src[loc[0]:end]); m != nil {
			path = m[1]
		}
		if _, ok := out[name]; !ok || path != "" {
			out[name] = path
		}
	}
	return out
}

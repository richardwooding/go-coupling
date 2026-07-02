package coupling

import (
	"os"
	"path/filepath"
	"strings"
)

// adapter encapsulates the per-ecosystem pieces the coupling metric needs; the
// graph math (in Graph.Coupling / Graph.Cycles) is language-agnostic. Adapters
// are stateful — prepare resolves and caches the first-party scope, then
// node/firstPartyImport consult it.
type adapter interface {
	// matchesLanguage reports whether a file's Language is one this adapter
	// analyses (the JS/TS adapter spans both).
	matchesLanguage(lang string) bool
	// prepare resolves the first-party scope rooted at root, caches per-language
	// state, and returns the project identity plus whether anything is
	// analysable. ok=false ⇒ an empty graph.
	prepare(root string) (module string, ok bool)
	// node maps a file to its node id, or "" to skip it.
	node(f File) string
	// firstPartyImport reports whether import imp (from a file whose node is
	// fromNode) targets a first-party node, returning that node id. nodes is the
	// set of every node the project's files occupy (the first-party boundary for
	// declaration-based adapters; manifest-based ones ignore it).
	firstPartyImport(imp, fromNode string, nodes map[string]bool) (node string, ok bool)
}

// adapterFor selects the adapter for a tree by its build manifest. Falls back
// to the Go adapter, whose prepare reports ok=false without a go.mod (an empty
// graph).
func adapterFor(root string) adapter {
	switch {
	case fileExists(filepath.Join(root, "go.mod")):
		return &goAdapter{}
	case fileExists(filepath.Join(root, "Cargo.toml")):
		return &rustAdapter{}
	case fileExists(filepath.Join(root, "Package.swift")):
		return &swiftAdapter{}
	case hasAnyFile(root,
		"pom.xml",
		"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts",
		"build.sbt",
		"build.sc", "build.mill"):
		return &packageDeclAdapter{langs: []string{"java", "kotlin", "scala"}, sep: "."}
	case isCSharpRoot(root):
		return &packageDeclAdapter{langs: []string{"csharp"}, sep: "."}
	case fileExists(filepath.Join(root, "composer.json")):
		return &packageDeclAdapter{langs: []string{"php"}, sep: "\\"}
	case hasAnyFile(root,
		"cpanfile", "Makefile.PL", "Build.PL",
		"dist.ini",
		"META.json", "META.yml", "MYMETA.json", "MYMETA.yml"):
		return &packageDeclAdapter{langs: []string{"perl"}, sep: "::"}
	case fileExists(filepath.Join(root, "Gemfile")) || hasGlobMatch(root, "*.gemspec"):
		return &rubyAdapter{}
	case hasAnyFile(root, "pyproject.toml", "setup.py", "setup.cfg", "Pipfile", "requirements.txt", "tox.ini"):
		return &pythonAdapter{}
	case hasAnyFile(root, "package.json", "tsconfig.json"):
		return &jstsAdapter{}
	case hasAnyFile(root, "CMakeLists.txt", "configure.ac", "configure.in", "meson.build", "Makefile.am", "GNUmakefile"):
		return &cppAdapter{}
	default:
		return &goAdapter{}
	}
}

// longestPackagePrefix resolves an import string to the first-party node that
// owns it: the longest prefix of the FQN (split on sep) that is a declared node.
func longestPackagePrefix(imp string, nodes map[string]bool, sep string) (string, bool) {
	p := strings.TrimSuffix(strings.TrimSpace(imp), ".*")
	p = strings.TrimPrefix(p, sep) // a PHP FQN may be written \App\... — declared nodes never lead with sep
	for p != "" {
		if nodes[p] {
			return p, true
		}
		i := strings.LastIndex(p, sep)
		if i <= 0 {
			break
		}
		p = p[:i]
	}
	return "", false
}

// moduledPath returns the module path declared in root/go.mod, or "".
func moduledPath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func hasAnyFile(dir string, names ...string) bool {
	for _, n := range names {
		if fileExists(filepath.Join(dir, n)) {
			return true
		}
	}
	return false
}

// hasGlobMatch reports whether dir contains a regular file whose basename
// matches pattern (e.g. "*.gemspec"). Matches basenames so glob metacharacters
// in the dir path itself can't break detection.
func hasGlobMatch(dir, pattern string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ok, _ := filepath.Match(pattern, e.Name()); ok {
			return true
		}
	}
	return false
}

// isCSharpRoot reports whether dir looks like a C#/.NET project root: a
// solution/project file or SDK-style marker, in dir or its immediate subdirs
// (solutions often live one level down).
func isCSharpRoot(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	var subdirs []string
	for _, e := range entries {
		if e.IsDir() {
			if !skipCargoDir(e.Name()) {
				subdirs = append(subdirs, e.Name())
			}
			continue
		}
		if isCSharpMarkerFile(e.Name()) {
			return true
		}
	}
	for _, sd := range subdirs {
		if csharpMarkersIn(filepath.Join(dir, sd)) {
			return true
		}
	}
	return false
}

func csharpMarkersIn(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && isCSharpMarkerFile(e.Name()) {
			return true
		}
	}
	return false
}

func isCSharpMarkerFile(name string) bool {
	switch name {
	case "Directory.Build.props", "Directory.Packages.props", "global.json":
		return true
	}
	switch filepath.Ext(name) {
	case ".sln", ".slnx", ".csproj":
		return true
	}
	return false
}

package coupling

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// rustAdapter computes crate-level coupling for a Cargo project or workspace.
// Nodes are crates; the first-party boundary is the set of workspace member
// crate names discovered from Cargo.toml manifests under the root. A
// `use <crate>::…` whose leading segment names a sibling member crate is an
// inter-crate edge; crate/self/super are intra-crate.
type rustAdapter struct {
	crates map[string]bool
	dirs   []crateDir
}

type crateDir struct {
	dir   string // absolute, cleaned manifest directory
	crate string // normalized crate name
}

func (a *rustAdapter) matchesLanguage(lang string) bool { return lang == "rust" }

func (a *rustAdapter) prepare(root string) (string, bool) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	a.crates = map[string]bool{}
	a.dirs = nil

	_ = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // skip unreadable entries, keep walking
		}
		if d.IsDir() {
			if path != absRoot && skipCargoDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() != "Cargo.toml" {
			return nil
		}
		name := cargoPackageName(path)
		if name == "" {
			return nil
		}
		norm := normalizeCrate(name)
		a.crates[norm] = true
		a.dirs = append(a.dirs, crateDir{dir: filepath.Dir(path), crate: norm})
		return nil
	})

	if len(a.crates) == 0 {
		return "", false
	}
	sort.Slice(a.dirs, func(i, j int) bool { return len(a.dirs[i].dir) > len(a.dirs[j].dir) })

	module := filepath.Base(absRoot)
	for _, cd := range a.dirs {
		if cd.dir == absRoot {
			module = cd.crate
			break
		}
	}
	return module, true
}

func (a *rustAdapter) node(f File) string {
	abs := f.Path
	if !filepath.IsAbs(abs) {
		if p, err := filepath.Abs(abs); err == nil {
			abs = p
		}
	}
	for _, cd := range a.dirs {
		if abs == cd.dir || strings.HasPrefix(abs, cd.dir+string(filepath.Separator)) {
			return cd.crate
		}
	}
	return ""
}

func (a *rustAdapter) firstPartyImport(imp, fromNode string, _ map[string]bool) (string, bool) {
	leading := imp
	if before, _, ok := strings.Cut(imp, "::"); ok {
		leading = before
	}
	leading = strings.TrimSpace(leading)
	switch leading {
	case "crate", "self", "super":
		return fromNode, true // intra-crate — no inter-crate edge
	}
	norm := normalizeCrate(leading)
	if a.crates[norm] {
		return norm, true
	}
	return "", false
}

// normalizeCrate maps a Cargo package name to its import-path form (hyphens
// become underscores: my-crate → my_crate).
func normalizeCrate(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "-", "_")
}

// cargoPackageName returns the [package] name in a Cargo.toml, or "".
func cargoPackageName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var manifest struct {
		Package struct {
			Name string `toml:"name"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return ""
	}
	return manifest.Package.Name
}

// skipCargoDir reports whether a directory should be pruned from the manifest
// walk (build output, VCS, vendored / hidden trees).
func skipCargoDir(name string) bool {
	switch name {
	case "target", ".git", "node_modules", "vendor":
		return true
	}
	return strings.HasPrefix(name, ".")
}

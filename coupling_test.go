package coupling_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	coupling "github.com/richardwooding/go-coupling"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func find(cs []coupling.Coupling, pkg string) (coupling.Coupling, bool) {
	for _, c := range cs {
		if c.Package == pkg {
			return c, true
		}
	}
	return coupling.Coupling{}, false
}

func mustCoupling(t *testing.T, cs []coupling.Coupling, pkg string, ca, ce int) {
	t.Helper()
	c, ok := find(cs, pkg)
	if !ok {
		t.Fatalf("package %q not found in %+v", pkg, cs)
	}
	if c.Afferent != ca || c.Efferent != ce {
		t.Errorf("%s: Ca=%d Ce=%d, want Ca=%d Ce=%d", pkg, c.Afferent, c.Efferent, ca, ce)
	}
}

func TestGoCoupling(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.com/m\n\ngo 1.23\n")
	files := []coupling.File{
		{Path: filepath.Join(root, "a", "a.go"), Language: "go", Imports: []string{"example.com/m/b"}},
		{Path: filepath.Join(root, "b", "b.go"), Language: "go", Imports: []string{"fmt"}}, // external, ignored
		{Path: filepath.Join(root, "c", "c.go"), Language: "go", Imports: []string{"example.com/m/a", "example.com/m/b"}},
	}
	g := coupling.Build(root, files)
	if g.Module() != "example.com/m" {
		t.Errorf("Module() = %q, want example.com/m", g.Module())
	}
	if !g.Analysable() {
		t.Fatal("graph not analysable")
	}
	cs := g.Coupling()
	mustCoupling(t, cs, "example.com/m/b", 2, 0) // imported by a and c
	mustCoupling(t, cs, "example.com/m/a", 1, 1) // imported by c; imports b
	mustCoupling(t, cs, "example.com/m/c", 0, 2) // imports a and b

	// Instability: b fully stable (0), c fully unstable (1).
	b, _ := find(cs, "example.com/m/b")
	if b.Instability != 0 {
		t.Errorf("b instability = %v, want 0", b.Instability)
	}
	c, _ := find(cs, "example.com/m/c")
	if c.Instability != 1 {
		t.Errorf("c instability = %v, want 1", c.Instability)
	}
	// Ranking: most-depended-upon (b, Ca=2) first.
	if cs[0].Package != "example.com/m/b" {
		t.Errorf("rank[0] = %q, want example.com/m/b", cs[0].Package)
	}
}

func TestGoCycle(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module m\n")
	files := []coupling.File{
		{Path: filepath.Join(root, "a", "a.go"), Language: "go", Imports: []string{"m/b"}},
		{Path: filepath.Join(root, "b", "b.go"), Language: "go", Imports: []string{"m/a"}},
		{Path: filepath.Join(root, "c", "c.go"), Language: "go", Imports: []string{"m/a"}}, // not in the cycle
	}
	cycles := coupling.Build(root, files).Cycles()
	if len(cycles) != 1 {
		t.Fatalf("got %d cycles, want 1: %+v", len(cycles), cycles)
	}
	if cycles[0].Length != 2 || !slices.Equal(cycles[0].Nodes, []string{"m/a", "m/b"}) {
		t.Errorf("cycle = %+v, want {m/a, m/b}", cycles[0])
	}
}

func TestJavaPackageDecl(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "pom.xml"), "<project/>\n")
	files := []coupling.File{
		{Path: "X.java", Language: "java", Package: "com.app.web", Imports: []string{"com.app.core.Service", "java.util.List"}},
		{Path: "Y.java", Language: "java", Package: "com.app.core", Imports: []string{}},
		{Path: "Z.java", Language: "java", Package: "com.app.api", Imports: []string{"com.app.core.Repo"}},
	}
	cs := coupling.Analyze(root, files)
	// core is imported by web and api (java.util.List is external → ignored).
	mustCoupling(t, cs, "com.app.core", 2, 0)
	mustCoupling(t, cs, "com.app.web", 0, 1)
	mustCoupling(t, cs, "com.app.api", 0, 1)
}

func TestPythonCoupling(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "pyproject.toml"), "[project]\nname = \"app\"\n")
	files := []coupling.File{
		{Path: filepath.Join(root, "app", "web", "v.py"), Language: "python", Imports: []string{"app.core"}},
		{Path: filepath.Join(root, "app", "core", "c.py"), Language: "python", Imports: []string{"os"}},
		// Relative import: from . import core, anchored at app.api → app.core sibling? use ..core
		{Path: filepath.Join(root, "app", "api", "a.py"), Language: "python", RelativeImports: []string{"..core"}},
	}
	cs := coupling.Analyze(root, files)
	mustCoupling(t, cs, "app.core", 2, 0) // imported by app.web (absolute) and app.api (relative ..core)
}

func TestRustCrateCoupling(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Cargo.toml"), "[package]\nname = \"app\"\n")
	write(t, filepath.Join(root, "crates", "util", "Cargo.toml"), "[package]\nname = \"app-util\"\n")
	files := []coupling.File{
		{Path: filepath.Join(root, "src", "main.rs"), Language: "rust", Imports: []string{"app_util::helper", "std::collections::HashMap"}},
		{Path: filepath.Join(root, "crates", "util", "src", "lib.rs"), Language: "rust", Imports: []string{"crate::internal"}},
	}
	cs := coupling.Analyze(root, files)
	// app_util imported by app (the std import is external; crate:: is intra).
	mustCoupling(t, cs, "app_util", 1, 0)
	mustCoupling(t, cs, "app", 0, 1)
}

func TestNoManifest(t *testing.T) {
	root := t.TempDir() // empty: falls back to Go adapter, no go.mod → not analysable
	files := []coupling.File{{Path: filepath.Join(root, "a.go"), Language: "go", Imports: []string{"x"}}}
	g := coupling.Build(root, files)
	if g.Analysable() {
		t.Error("expected non-analysable graph for a root with no manifest")
	}
	if len(g.Coupling()) != 0 || len(g.Cycles()) != 0 {
		t.Error("expected empty coupling/cycles for non-analysable graph")
	}
}

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

func TestFileCoupling(t *testing.T) {
	// Path-based ecosystem (Go): each file's profile is its directory node's.
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.com/m\n\ngo 1.23\n")
	aPath := filepath.Join(root, "a", "a.go")
	cPath := filepath.Join(root, "c", "c.go")
	files := []coupling.File{
		{Path: aPath, Language: "go", Imports: []string{"example.com/m/b"}},
		{Path: filepath.Join(root, "b", "b.go"), Language: "go", Imports: []string{"fmt"}},
		{Path: cPath, Language: "go", Imports: []string{"example.com/m/a", "example.com/m/b"}},
	}
	fc := coupling.Build(root, files).FileCoupling()
	if c, ok := fc[cPath]; !ok || c.Afferent != 0 || c.Efferent != 2 {
		t.Errorf("c.go coupling = %+v ok=%v, want Ca=0 Ce=2", c, ok)
	}
	if c, ok := fc[aPath]; !ok || c.Afferent != 1 || c.Efferent != 1 {
		t.Errorf("a.go coupling = %+v ok=%v, want Ca=1 Ce=1", c, ok)
	}

	// Declaration-based ecosystem (Java): key echoes the Path given verbatim,
	// and files in the same package share one node's profile.
	jroot := t.TempDir()
	write(t, filepath.Join(jroot, "pom.xml"), "<project/>\n")
	jfiles := []coupling.File{
		{Path: "X.java", Language: "java", Package: "com.app.web", Imports: []string{"com.app.core.Service"}},
		{Path: "Y.java", Language: "java", Package: "com.app.core"},
		{Path: "Z.java", Language: "java", Package: "com.app.core"},
	}
	jfc := coupling.Build(jroot, jfiles).FileCoupling()
	y, yok := jfc["Y.java"]
	z, zok := jfc["Z.java"]
	if !yok || !zok || y != z || y.Afferent != 1 {
		t.Errorf("Y/Z share com.app.core (Ca=1): Y=%+v(%v) Z=%+v(%v)", y, yok, z, zok)
	}

	// Non-analysable root: no manifest, so nothing maps.
	if got := coupling.Build(t.TempDir(), files).FileCoupling(); len(got) != 0 {
		t.Errorf("non-analysable FileCoupling = %v, want empty", got)
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

func TestSwiftModuleCoupling(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Package.swift"),
		"// swift-tools-version:5.9\nimport PackageDescription\n"+
			"let package = Package(name: \"App\", targets: [\n"+
			"  .executableTarget(name: \"App\", dependencies: [\"Svc\", \"Util\"]),\n"+
			"  .target(name: \"Svc\", dependencies: [\"Util\"]),\n"+
			"  .target(name: \"Util\"),\n"+
			"])\n")
	// Source files under the conventional Sources/<Target>/ layout.
	write(t, filepath.Join(root, "Sources", "App", "main.swift"), "import Svc\nimport Util\nimport Foundation\n")
	write(t, filepath.Join(root, "Sources", "Svc", "Svc.swift"), "import Util.Networking\n")
	write(t, filepath.Join(root, "Sources", "Util", "Util.swift"), "import Foundation\n")
	files := []coupling.File{
		{Path: filepath.Join(root, "Sources", "App", "main.swift"), Language: "swift", Imports: []string{"Svc", "Util", "Foundation"}},
		{Path: filepath.Join(root, "Sources", "Svc", "Svc.swift"), Language: "swift", Imports: []string{"Util.Networking"}},
		{Path: filepath.Join(root, "Sources", "Util", "Util.swift"), Language: "swift", Imports: []string{"Foundation"}},
	}
	cs := coupling.Analyze(root, files)
	// Foundation is external → ignored; Util.Networking resolves to module Util.
	mustCoupling(t, cs, "Util", 2, 0) // imported by App and Svc
	mustCoupling(t, cs, "Svc", 1, 1)  // imported by App; imports Util
	mustCoupling(t, cs, "App", 0, 2)  // imports Svc and Util
}

func TestSwiftExplicitPath(t *testing.T) {
	root := t.TempDir()
	// A target whose sources live outside Sources/ via an explicit path:.
	write(t, filepath.Join(root, "Package.swift"),
		"let package = Package(name: \"P\", targets: [\n"+
			"  .target(name: \"Core\", path: \"custom/core\"),\n"+
			"  .target(name: \"Api\", dependencies: [\"Core\"]),\n"+
			"])\n")
	write(t, filepath.Join(root, "custom", "core", "c.swift"), "import Foundation\n")
	write(t, filepath.Join(root, "Sources", "Api", "a.swift"), "import Core\n")
	files := []coupling.File{
		{Path: filepath.Join(root, "custom", "core", "c.swift"), Language: "swift", Imports: []string{"Foundation"}},
		{Path: filepath.Join(root, "Sources", "Api", "a.swift"), Language: "swift", Imports: []string{"Core"}},
	}
	cs := coupling.Analyze(root, files)
	mustCoupling(t, cs, "Core", 1, 0)
	mustCoupling(t, cs, "Api", 0, 1)
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

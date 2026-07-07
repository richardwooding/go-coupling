# go-coupling

[![Go Reference](https://pkg.go.dev/badge/github.com/richardwooding/go-coupling.svg)](https://pkg.go.dev/github.com/richardwooding/go-coupling)

**Website:** [richardwooding.github.io/go-coupling](https://richardwooding.github.io/go-coupling/)

Robert C. Martin's package-coupling metrics — **afferent coupling (Ca)**,
**efferent coupling (Ce)**, **instability (I = Ce/(Ca+Ce))** — plus **circular
dependency** detection, across **9 language ecosystems**.

No other open-source Go library computes these metrics with manifest-aware,
multi-language first-party detection.

| Ecosystem | Detected by | Node = |
|---|---|---|
| Go | `go.mod` | package import path |
| Rust | `Cargo.toml` | crate |
| Swift | `Package.swift` | SwiftPM target/module |
| JVM (Java/Kotlin/Scala) | `pom.xml` / Gradle / sbt / Mill | declared package |
| C# | `.sln` / `.csproj` / SDK markers | namespace |
| PHP | `composer.json` | namespace (`\`-separated) |
| Perl | `cpanfile` / `Makefile.PL` / `dist.ini` / META | package (`::`-separated) |
| Python | `pyproject.toml` / `setup.py` / … | dotted package dir |
| JS / TS | `package.json` / `tsconfig.json` | directory module |
| C / C++ | CMake / autotools / meson | directory module (`#include` graph) |

## How it works

go-coupling does **no parsing**. You give it, per file, the data you've already
extracted — path, language, imports, declared package — and a project root. It
detects the ecosystem from the root's build manifest, builds the first-party
import graph, and computes the metrics.

Pair it with [`treesitter-symbols`][tss] (or any extractor) to get the imports
and package per file:

```go
import (
	coupling "github.com/richardwooding/go-coupling"
	symbols  "github.com/richardwooding/treesitter-symbols"
)

var files []coupling.File
for _, path := range sourcePaths {
	src, _ := os.ReadFile(path)
	s, _ := symbols.Extract(lang, src)
	files = append(files, coupling.File{
		Path: path, Language: lang,
		Imports: s.Imports, RelativeImports: s.RelativeImports, Package: s.Package,
	})
}

g := coupling.Build(root, files)
for _, c := range g.Coupling() {
	fmt.Printf("%-30s Ca=%d Ce=%d I=%.2f\n", c.Package, c.Afferent, c.Efferent, c.Instability)
}
for _, cyc := range g.Cycles() {
	fmt.Printf("cycle: %v\n", cyc.Nodes)
}
```

`Coupling()` is ranked most-depended-upon first (high Ca), then most unstable
(high I) — the fragile hubs where a refactor is riskiest. `Cycles()` returns
strongly-connected components of size > 1, largest first.

`FileCoupling()` maps each file's `Path` (as given to `Build`) to its node's
Ca/Ce/I, so you can attribute node-level coupling back to individual files
across **any** supported ecosystem without reproducing its node rule:

```go
for path, c := range g.FileCoupling() {
	fmt.Printf("%-40s Ca=%d Ce=%d\n", path, c.Afferent, c.Efferent)
}
```

Convenience wrappers: `Analyze(root, files)` and `FindCycles(root, files)`.

## Install

```sh
go get github.com/richardwooding/go-coupling
```

The only dependency is `github.com/pelletier/go-toml/v2` (for `Cargo.toml`).

## Notes

- `Build` reads the filesystem at `root` to detect the ecosystem and resolve
  first-party boundaries (Rust crate manifests, C/C++ include roots, the Python
  `src/` and Ruby `lib/` layouts). `File.Path` may be absolute or relative to
  the current working directory.
- Resolution is name/structure-based and static: dynamic `require`, autoloading
  (Ruby Zeitwerk), and tsconfig path aliases aren't followed — documented
  per-adapter limitations shared with the source this was extracted from.
- A root with no recognised manifest yields a non-analysable graph (empty
  results), not an error.

## License

MIT — see [LICENSE](LICENSE).

[tss]: https://github.com/richardwooding/treesitter-symbols

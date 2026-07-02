// Package coupling computes Robert C. Martin's package-coupling metrics —
// afferent coupling (Ca), efferent coupling (Ce), and instability
// (I = Ce/(Ca+Ce)) — and detects circular dependencies, across many language
// ecosystems.
//
// It works on a first-party import graph it builds from per-file data the
// caller supplies (path, language, imports, declared package). The package
// itself does no parsing; pair it with a symbol extractor such as
// github.com/richardwooding/treesitter-symbols to obtain the imports and
// package for each file.
//
// The ecosystem is detected from the build manifest at the project root —
// go.mod (Go packages), Cargo.toml (Rust crates), Package.swift (SwiftPM
// target modules), Maven/Gradle/sbt (JVM
// packages), .sln/.csproj (C# namespaces), a Python manifest (packages),
// package.json/tsconfig.json (JS/TS directory modules), a Perl dist manifest
// (:: packages), Gemfile/*.gemspec (Ruby directory modules), and
// CMake/autotools/meson (C/C++ #include directory modules). The first-party
// boundary and the file→node / import→node mappings are per-ecosystem; the
// graph math is language-agnostic.
//
//	g := coupling.Build(root, files)
//	for _, c := range g.Coupling() { ... } // Ca / Ce / I per package
//	for _, c := range g.Cycles()  { ... } // circular dependencies
package coupling

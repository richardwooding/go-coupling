package coupling

import "sort"

// File is the per-file input to [Build]: the data a caller has already
// extracted for one source file. Path may be absolute or relative to the
// current working directory.
type File struct {
	// Path is the file's location on disk (absolute or cwd-relative). Used by
	// path-based ecosystems (Go/Rust/Python/JS-TS/Ruby/C-C++) to map a file to
	// its node.
	Path string
	// Language is the canonical language id ("go", "rust", "java", …).
	Language string
	// Imports are the file's import paths (absolute form).
	Imports []string
	// RelativeImports are dotted-relative imports with leading dots preserved
	// (Python); empty for other languages.
	RelativeImports []string
	// Package is the file's declared package / namespace, for the
	// declaration-based ecosystems (Java/Kotlin/Scala/C#/PHP/Perl). Ignored by
	// the path-based ones.
	Package string
}

// Coupling is the afferent/efferent coupling profile of one first-party node
// (package / crate / namespace / directory module).
type Coupling struct {
	Package     string  `json:"package"`     // node id (import path / crate / namespace / dir)
	Afferent    int     `json:"afferent"`    // Ca: distinct first-party nodes that import this one
	Efferent    int     `json:"efferent"`    // Ce: distinct first-party nodes this one imports
	Instability float64 `json:"instability"` // I = Ce / (Ca + Ce); 0 when isolated
}

// Cycle is one circular dependency: a strongly-connected component of size > 1
// in the first-party import graph. Nodes is the sorted member set (not an
// edge-ordered path).
type Cycle struct {
	Nodes  []string `json:"nodes"`
	Length int      `json:"length"`
}

// Graph is a first-party import graph built by [Build]. Query it with
// [Graph.Coupling] and [Graph.Cycles]; both are pure reads, so a Graph may be
// queried repeatedly and concurrently.
type Graph struct {
	module     string
	analysable bool
	efferent   map[string]map[string]bool // node -> set of nodes it imports
	afferent   map[string]map[string]bool // node -> set of nodes importing it
	fileNodes  map[string]string          // File.Path -> node it maps to (first-party files only)
}

// Build constructs the first-party import graph for the project rooted at root
// from files. root selects the ecosystem (via its build manifest) and anchors
// first-party resolution; each File carries the imports / package the caller
// already extracted.
//
// When root carries no recognised manifest, the returned graph is non-analysable
// ([Graph.Analysable] is false) and its queries return empty — mirroring the
// "unknown ecosystem" case rather than erroring.
func Build(root string, files []File) *Graph {
	if root == "" {
		root = "."
	}
	g := &Graph{
		efferent:  map[string]map[string]bool{},
		afferent:  map[string]map[string]bool{},
		fileNodes: map[string]string{},
	}
	ad := adapterFor(root)
	module, ok := ad.prepare(root)
	g.module = module
	if !ok {
		return g
	}
	g.analysable = true

	ensure := func(m map[string]map[string]bool, k string) map[string]bool {
		if m[k] == nil {
			m[k] = map[string]bool{}
		}
		return m[k]
	}
	touch := func(node string) {
		ensure(g.efferent, node)
		ensure(g.afferent, node)
	}

	// Pass 1: map each first-party file to its node and collect the node set.
	fileNode := make([]string, len(files))
	nodes := map[string]bool{}
	for i, f := range files {
		if !ad.matchesLanguage(f.Language) {
			continue
		}
		node := ad.node(f)
		if node == "" {
			continue
		}
		fileNode[i] = node
		g.fileNodes[f.Path] = node
		nodes[node] = true
		touch(node)
	}

	addEdges := func(node string, imports []string) {
		for _, imp := range imports {
			target, ok := ad.firstPartyImport(imp, node, nodes)
			if !ok || target == node {
				continue
			}
			touch(target)
			g.efferent[node][target] = true
			g.afferent[target][node] = true
		}
	}

	// Pass 2: resolve each file's imports to first-party nodes and record edges.
	for i, f := range files {
		node := fileNode[i]
		if node == "" {
			continue
		}
		addEdges(node, f.Imports)
		addEdges(node, f.RelativeImports)
	}
	return g
}

// Module returns the project identity: the go.mod module path, the Rust
// workspace root crate, or the root directory's base name for the
// declaration/directory ecosystems. "" when the ecosystem is unknown.
func (g *Graph) Module() string { return g.module }

// Analysable reports whether root carried a recognised build manifest. When
// false, Coupling and Cycles return empty.
func (g *Graph) Analysable() bool { return g.analysable }

// Coupling returns the per-node Ca/Ce/I profile, ranked most-depended-upon
// first (high Ca), then most unstable (high I), then by name — the "fragile
// hub" seams where a refactor is riskiest.
func (g *Graph) Coupling() []Coupling {
	out := make([]Coupling, 0, len(g.efferent))
	for node := range g.efferent {
		ca, ce := len(g.afferent[node]), len(g.efferent[node])
		inst := 0.0
		if ca+ce > 0 {
			inst = float64(ce) / float64(ca+ce)
		}
		out = append(out, Coupling{Package: node, Afferent: ca, Efferent: ce, Instability: inst})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Afferent != b.Afferent {
			return a.Afferent > b.Afferent
		}
		if a.Instability != b.Instability {
			return a.Instability > b.Instability
		}
		return a.Package < b.Package
	})
	return out
}

// FileCoupling returns the coupling profile of each first-party file's node,
// keyed by the File.Path that was passed to [Build]. Files that mapped to no
// first-party node — an unmatched language, an empty/external node, or a file
// under a non-analysable root — are absent from the map.
//
// It lets a caller attribute node-level Ca/Ce/instability back to individual
// files (many files may share one node's profile) without reproducing the
// ecosystem's node rule. Use the same Path form (absolute or relative) you gave
// Build; the returned keys echo it verbatim.
func (g *Graph) FileCoupling() map[string]Coupling {
	byNode := make(map[string]Coupling, len(g.efferent))
	for _, c := range g.Coupling() {
		byNode[c.Package] = c
	}
	out := make(map[string]Coupling, len(g.fileNodes))
	for path, node := range g.fileNodes {
		if c, ok := byNode[node]; ok {
			out[path] = c
		}
	}
	return out
}

// Cycles returns the circular dependencies — strongly-connected components of
// size > 1 — ranked largest-first, then alphabetically by first member.
func (g *Graph) Cycles() []Cycle {
	var out []Cycle
	for _, comp := range tarjanSCC(g.efferent) {
		if len(comp) < 2 {
			continue
		}
		sort.Strings(comp)
		out = append(out, Cycle{Nodes: comp, Length: len(comp)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Length != out[j].Length {
			return out[i].Length > out[j].Length
		}
		return out[i].Nodes[0] < out[j].Nodes[0]
	})
	return out
}

// Analyze is a convenience wrapper: Build(root, files).Coupling().
func Analyze(root string, files []File) []Coupling {
	return Build(root, files).Coupling()
}

// FindCycles is a convenience wrapper: Build(root, files).Cycles().
func FindCycles(root string, files []File) []Cycle {
	return Build(root, files).Cycles()
}

// tarjanSCC returns the strongly-connected components of a directed graph given
// as an adjacency map (node → set of successors). Iteration is sorted, so the
// output is deterministic. Every successor is assumed to be a key in adj (Build
// touches every node).
func tarjanSCC(adj map[string]map[string]bool) [][]string {
	nodes := make([]string, 0, len(adj))
	for n := range adj {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	successors := func(n string) []string {
		s := make([]string, 0, len(adj[n]))
		for m := range adj[n] {
			s = append(s, m)
		}
		sort.Strings(s)
		return s
	}

	index := make(map[string]int, len(nodes))
	lowlink := make(map[string]int, len(nodes))
	onStack := make(map[string]bool, len(nodes))
	var stack []string
	var counter int
	var out [][]string

	var strongConnect func(v string)
	strongConnect = func(v string) {
		index[v] = counter
		lowlink[v] = counter
		counter++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range successors(v) {
			if _, seen := index[w]; !seen {
				strongConnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				if index[w] < lowlink[v] {
					lowlink[v] = index[w]
				}
			}
		}

		if lowlink[v] == index[v] {
			var comp []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp = append(comp, w)
				if w == v {
					break
				}
			}
			out = append(out, comp)
		}
	}

	for _, n := range nodes {
		if _, seen := index[n]; !seen {
			strongConnect(n)
		}
	}
	return out
}

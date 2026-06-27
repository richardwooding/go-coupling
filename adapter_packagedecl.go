package coupling

import (
	"path/filepath"
	"slices"
)

// packageDeclAdapter computes package-level coupling for languages where each
// file declares its package / namespace in source and the first-party boundary
// is the set of packages the tree itself declares. The JVM family (Java /
// Kotlin / Scala — `package com.foo.bar`), C# (`namespace Foo.Bar`), PHP
// (`namespace App\Services`), and Perl (`package Foo::Bar`) share this model;
// they differ only in the namespace separator.
//
//   - node  = the file's declared package (File.Package);
//   - import resolves to the longest declared-package prefix of its FQN.
type packageDeclAdapter struct {
	langs  []string // languages this adapter analyses
	sep    string   // namespace separator: "." (JVM/C#), "\\" (PHP), "::" (Perl)
	module string
}

func (a *packageDeclAdapter) matchesLanguage(lang string) bool {
	return slices.Contains(a.langs, lang)
}

func (a *packageDeclAdapter) prepare(root string) (string, bool) {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	a.module = filepath.Base(abs)
	return a.module, true
}

// node returns the file's declared package / namespace. Files with none (Java
// default package, C# top-level statements) are skipped.
func (a *packageDeclAdapter) node(f File) string {
	return f.Package
}

// firstPartyImport maps an import FQN to the first-party package that owns it
// via the longest declared-package prefix (covers plain, static, and wildcard
// import forms).
func (a *packageDeclAdapter) firstPartyImport(imp, _ string, nodes map[string]bool) (string, bool) {
	return longestPackagePrefix(imp, nodes, a.sep)
}

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// symbolSet is the exported surface of the generated files of lib/proto:
// every top-level identifier with the domain of the file it sits in, every
// exported struct field, and which of them are deprecated. Two sets, read
// before and after a regeneration, give the symbol-level summary a Roll pull
// request carries (ADR-0004).
type symbolSet struct {
	domain     map[string]string // identifier -> domain; "" in definitions.go
	fields     map[string]bool   // "Type.Field"
	deprecated map[string]bool   // identifier or "Type.Field"
}

func newSymbolSet() *symbolSet {
	return &symbolSet{
		domain:     map[string]string{},
		fields:     map[string]bool{},
		deprecated: map[string]bool{},
	}
}

// readSymbols parses the generated files of dir: every .go file that is not
// hand-written (a_ prefix) and not a test.
func readSymbols(dir string) (*symbolSet, error) {
	set := newSymbolSet()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasPrefix(name, "a_") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		set.add(f)
	}

	return set, nil
}

func (s *symbolSet) add(f *ast.File) {
	domain := fileDomain(f)

	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			switch spec := spec.(type) {
			case *ast.TypeSpec:
				s.addType(spec, gen.Doc, domain)
			case *ast.ValueSpec:
				doc := spec.Doc
				if doc == nil {
					doc = gen.Doc
				}
				for _, n := range spec.Names {
					if n.IsExported() {
						s.addName(n.Name, domain, doc)
					}
				}
			}
		}
	}
}

func (s *symbolSet) addType(spec *ast.TypeSpec, genDoc *ast.CommentGroup, domain string) {
	if !spec.Name.IsExported() {
		return
	}
	doc := spec.Doc
	if doc == nil {
		doc = genDoc
	}
	s.addName(spec.Name.Name, domain, doc)

	st, ok := spec.Type.(*ast.StructType)
	if !ok {
		return
	}
	for _, field := range st.Fields.List {
		for _, n := range field.Names {
			if !n.IsExported() {
				continue
			}
			key := spec.Name.Name + "." + n.Name
			s.fields[key] = true
			if isDeprecated(field.Doc) {
				s.deprecated[key] = true
			}
		}
	}
}

func (s *symbolSet) addName(name, domain string, doc *ast.CommentGroup) {
	s.domain[name] = domain
	if isDeprecated(doc) {
		s.deprecated[name] = true
	}
}

// fileDomain is the domain a generated file holds, the first line of its
// leading block comment; definitions.go has none.
func fileDomain(f *ast.File) string {
	for _, c := range f.Comments {
		if len(c.List) == 0 || !strings.HasPrefix(c.List[0].Text, "/*") {
			continue
		}
		first, _, _ := strings.Cut(strings.TrimSpace(c.Text()), "\n")
		return strings.TrimSpace(first)
	}
	return ""
}

// isDeprecated reports a Go Deprecated paragraph, or the "(deprecated)"
// prefix upstream's generator wrote before the paragraph existed.
func isDeprecated(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, line := range strings.Split(doc.Text(), "\n") {
		if strings.HasPrefix(line, "Deprecated:") || strings.Contains(line, "(deprecated)") {
			return true
		}
	}
	return false
}

// summary is what changed between two symbol sets, as Go identifiers.
type summary struct {
	added      int
	removed    []string // identifiers and "Type.Field"
	renamed    [][2]string
	deprecated []string // newly deprecated identifiers and "Type.Field"
}

// summarize compares the committed surface with the regenerated one. A
// removed identifier and an added one that share their name after the
// domain prefix are a rename, such as a type that moved to another domain.
// Fields are reported under their type's current name and only while the
// type itself survives; the type's removal covers the rest.
func summarize(before, after *symbolSet) summary {
	removed, added := diffNames(before, after)
	renamedTo, renamed := pairRenames(removed, added, before, after)

	s := summary{added: len(added), renamed: renamed}
	for _, old := range removed {
		if _, has := renamedTo[old]; !has {
			s.removed = append(s.removed, old)
		}
	}
	s.removed = append(s.removed, removedFields(before, after, renamedTo)...)
	s.deprecated = newlyDeprecated(before, after, renamedTo)

	sort.Strings(s.removed)
	sort.Strings(s.deprecated)

	return s
}

// diffNames is the top-level identifiers only before has and only after
// has, sorted.
func diffNames(before, after *symbolSet) (removed, added []string) {
	removed, added = []string{}, []string{}
	for name := range before.domain {
		if _, has := after.domain[name]; !has {
			removed = append(removed, name)
		}
	}
	for name := range after.domain {
		if _, has := before.domain[name]; !has {
			added = append(added, name)
		}
	}
	sort.Strings(removed)
	sort.Strings(added)
	return removed, added
}

// pairRenames matches each removed identifier with the one added identifier
// that carries the same name after its domain prefix; two candidates or
// none leave it a removal.
func pairRenames(removed, added []string, before, after *symbolSet) (map[string]string, [][2]string) {
	renamedTo := map[string]string{}
	renamed := [][2]string{}
	taken := map[string]bool{}

	for _, old := range removed {
		suffix := strings.TrimPrefix(old, before.domain[old])
		candidate := ""
		for _, name := range added {
			if taken[name] || strings.TrimPrefix(name, after.domain[name]) != suffix {
				continue
			}
			if candidate != "" {
				candidate = ""
				break
			}
			candidate = name
		}
		if candidate != "" {
			renamedTo[old] = candidate
			taken[candidate] = true
			renamed = append(renamed, [2]string{old, candidate})
		}
	}

	return renamedTo, renamed
}

// currentName maps an identifier or a "Type.Field" key through the renames
// to its name after the regeneration.
func currentName(renamedTo map[string]string, key string) string {
	typ, field, isField := strings.Cut(key, ".")
	if to, has := renamedTo[typ]; has {
		typ = to
	}
	if isField {
		return typ + "." + field
	}
	return typ
}

// removedFields is the fields before has whose type survives, under its
// current name, but which after lacks.
func removedFields(before, after *symbolSet, renamedTo map[string]string) []string {
	list := []string{}
	for key := range before.fields {
		typ, _, _ := strings.Cut(key, ".")
		if _, has := after.domain[currentName(renamedTo, typ)]; !has {
			continue
		}
		if now := currentName(renamedTo, key); !after.fields[now] {
			list = append(list, now)
		}
	}
	return list
}

// newlyDeprecated is what after marks deprecated and before did not, under
// the current names.
func newlyDeprecated(before, after *symbolSet, renamedTo map[string]string) []string {
	was := map[string]bool{}
	for old := range before.deprecated {
		was[currentName(renamedTo, old)] = true
	}

	list := []string{}
	for key := range after.deprecated {
		typ, _, _ := strings.Cut(key, ".")
		if _, has := after.domain[typ]; !has || was[key] {
			continue
		}
		list = append(list, key)
	}
	return list
}

// markdown renders the summary for a pull-request body.
func (s summary) markdown(roll int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Protocol roll r%d: %d Go identifiers removed, %d renamed, %d newly deprecated, %d added.\n",
		roll, len(s.removed), len(s.renamed), len(s.deprecated), s.added)

	section := func(title string, items []string) {
		fmt.Fprintf(&b, "\n%s:", title)
		if len(items) == 0 {
			b.WriteString(" none\n")
			return
		}
		b.WriteString("\n")
		for _, item := range items {
			fmt.Fprintf(&b, "- `%s`\n", item)
		}
	}

	section("Removed", s.removed)

	renamed := []string{}
	for _, r := range s.renamed {
		renamed = append(renamed, r[0]+"` -> `"+r[1])
	}
	section("Renamed", renamed)

	section("Newly deprecated", s.deprecated)

	return b.String()
}

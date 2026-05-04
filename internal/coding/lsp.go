// Package coding's LSP-style runtime gives the coding agent a local,
// dependency-free code-intelligence layer for Go workspaces. The intent
// is to surface enough structure (definitions, references, symbols,
// hover, diagnostics) to feed the agent's context without paying the
// startup cost of a full external language server. Other languages
// (TypeScript, Python, Rust) plug in by implementing the LSPProvider
// interface; the Go provider here is the reference implementation.
package coding

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// LSPLocation is a single source location: 1-indexed line, 1-indexed column.
type LSPLocation struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
}

// LSPSymbol is one named declaration. Kind is "func", "type", "var",
// "const", or "method".
type LSPSymbol struct {
	Name      string      `json:"name"`
	Kind      string      `json:"kind"`
	Container string      `json:"container,omitempty"`
	Location  LSPLocation `json:"location"`
}

// LSPHover is the documentation/comment block attached to a declaration.
type LSPHover struct {
	Symbol    LSPSymbol `json:"symbol"`
	Signature string    `json:"signature"`
	Doc       string    `json:"doc,omitempty"`
}

// LSPDiagnostic is one parser-level error/warning.
type LSPDiagnostic struct {
	Location LSPLocation `json:"location"`
	Severity string      `json:"severity"` // "error" | "warning"
	Message  string      `json:"message"`
}

// LSPProvider is the language-agnostic contract. New languages plug in
// by implementing this interface; the agent loop selects providers
// based on file extension.
type LSPProvider interface {
	Languages() []string
	Symbols(workspaceRoot string) ([]LSPSymbol, error)
	Definition(workspaceRoot, name string) ([]LSPLocation, error)
	References(workspaceRoot, name string) ([]LSPLocation, error)
	Hover(workspaceRoot, name string) (LSPHover, bool, error)
	Diagnostics(workspaceRoot string) ([]LSPDiagnostic, error)
}

// LSPRuntime owns the registered providers and dispatches by file
// extension. A single workspace root is queried at a time.
type LSPRuntime struct {
	mu        sync.RWMutex
	providers map[string]LSPProvider // ext -> provider
}

// NewLSPRuntime returns a runtime with the Go provider pre-registered.
func NewLSPRuntime() *LSPRuntime {
	r := &LSPRuntime{providers: map[string]LSPProvider{}}
	r.Register(&GoLSPProvider{})
	return r
}

// Register adds a provider for every language extension it claims.
func (r *LSPRuntime) Register(p LSPProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, lang := range p.Languages() {
		r.providers[lang] = p
	}
}

// ProviderFor returns the provider for the given file extension, or nil.
func (r *LSPRuntime) ProviderFor(ext string) LSPProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[strings.ToLower(strings.TrimPrefix(ext, "."))]
}

// Symbols aggregates symbols from every registered provider for the
// workspace. Errors from a single provider are collected and returned
// alongside whatever symbols were gathered, so a parse error in one
// file doesn't blank the whole index.
func (r *LSPRuntime) Symbols(workspace string) ([]LSPSymbol, error) {
	r.mu.RLock()
	provs := uniqueProviders(r.providers)
	r.mu.RUnlock()
	var out []LSPSymbol
	var firstErr error
	for _, p := range provs {
		got, err := p.Symbols(workspace)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		out = append(out, got...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Location.Path != out[j].Location.Path {
			return out[i].Location.Path < out[j].Location.Path
		}
		return out[i].Location.Line < out[j].Location.Line
	})
	return out, firstErr
}

// Diagnostics aggregates diagnostics across providers.
func (r *LSPRuntime) Diagnostics(workspace string) ([]LSPDiagnostic, error) {
	r.mu.RLock()
	provs := uniqueProviders(r.providers)
	r.mu.RUnlock()
	var out []LSPDiagnostic
	var firstErr error
	for _, p := range provs {
		got, err := p.Diagnostics(workspace)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		out = append(out, got...)
	}
	return out, firstErr
}

func uniqueProviders(m map[string]LSPProvider) []LSPProvider {
	seen := map[LSPProvider]bool{}
	var out []LSPProvider
	for _, p := range m {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// GoLSPProvider implements LSPProvider for Go source files using only
// the standard library's go/parser. It walks the workspace once and
// caches per-call; callers that need long-lived state can wrap it in
// their own caching layer.
type GoLSPProvider struct{}

// Languages returns the file extensions this provider claims.
func (*GoLSPProvider) Languages() []string { return []string{"go"} }

// Symbols returns every top-level declaration plus methods.
func (g *GoLSPProvider) Symbols(workspace string) ([]LSPSymbol, error) {
	var out []LSPSymbol
	err := walkGoFiles(workspace, func(path string, file *ast.File, fset *token.FileSet) {
		for _, decl := range file.Decls {
			out = append(out, declSymbols(path, fset, decl)...)
		}
	})
	return out, err
}

// Definition returns locations declaring a symbol with the given name.
func (g *GoLSPProvider) Definition(workspace, name string) ([]LSPLocation, error) {
	var out []LSPLocation
	err := walkGoFiles(workspace, func(path string, file *ast.File, fset *token.FileSet) {
		for _, decl := range file.Decls {
			for _, sym := range declSymbols(path, fset, decl) {
				if sym.Name == name {
					out = append(out, sym.Location)
				}
			}
		}
	})
	return out, err
}

// References returns locations where the named identifier is used (as
// any *ast.Ident with a matching Name). Imports and shadowed locals are
// included — the agent uses the result as a starting list, not a final
// answer.
func (g *GoLSPProvider) References(workspace, name string) ([]LSPLocation, error) {
	var out []LSPLocation
	err := walkGoFiles(workspace, func(path string, file *ast.File, fset *token.FileSet) {
		ast.Inspect(file, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok || ident.Name != name {
				return true
			}
			pos := fset.Position(ident.Pos())
			out = append(out, LSPLocation{Path: path, Line: pos.Line, Col: pos.Column})
			return true
		})
	})
	return out, err
}

// Hover returns the doc comment + signature for the first declaration
// matching name.
func (g *GoLSPProvider) Hover(workspace, name string) (LSPHover, bool, error) {
	var out LSPHover
	var found bool
	err := walkGoFiles(workspace, func(path string, file *ast.File, fset *token.FileSet) {
		if found {
			return
		}
		for _, decl := range file.Decls {
			for _, sym := range declSymbols(path, fset, decl) {
				if sym.Name != name {
					continue
				}
				out = LSPHover{Symbol: sym, Signature: declSignature(decl), Doc: declDoc(decl)}
				found = true
				return
			}
		}
	})
	return out, found, err
}

// Diagnostics returns parser-level errors and warnings.
func (g *GoLSPProvider) Diagnostics(workspace string) ([]LSPDiagnostic, error) {
	var out []LSPDiagnostic
	fset := token.NewFileSet()
	err := filepath.WalkDir(workspace, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		_, perr := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.AllErrors)
		if perr == nil {
			return nil
		}
		// parser returns scanner.ErrorList — extract per-position entries.
		out = append(out, errorsToDiagnostics(perr)...)
		return nil
	})
	return out, err
}

// walkGoFiles parses every .go file under root and invokes visit. Parse
// errors are silently skipped — Diagnostics surfaces them separately.
func walkGoFiles(root string, visit func(path string, file *ast.File, fset *token.FileSet)) error {
	if _, err := os.Stat(root); err != nil {
		return fmt.Errorf("lsp: workspace %q: %w", root, err)
	}
	fset := token.NewFileSet()
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}
		visit(path, file, fset)
		return nil
	})
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", ".conduit":
		return true
	}
	return strings.HasPrefix(name, ".")
}

func declSymbols(path string, fset *token.FileSet, decl ast.Decl) []LSPSymbol {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		kind := "func"
		container := ""
		if d.Recv != nil && len(d.Recv.List) > 0 {
			kind = "method"
			container = receiverTypeName(d.Recv.List[0])
		}
		pos := fset.Position(d.Name.Pos())
		return []LSPSymbol{{
			Name: d.Name.Name, Kind: kind, Container: container,
			Location: LSPLocation{Path: path, Line: pos.Line, Col: pos.Column},
		}}
	case *ast.GenDecl:
		var out []LSPSymbol
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				pos := fset.Position(s.Name.Pos())
				out = append(out, LSPSymbol{
					Name: s.Name.Name, Kind: "type",
					Location: LSPLocation{Path: path, Line: pos.Line, Col: pos.Column},
				})
			case *ast.ValueSpec:
				kind := "var"
				if d.Tok == token.CONST {
					kind = "const"
				}
				for _, n := range s.Names {
					pos := fset.Position(n.Pos())
					out = append(out, LSPSymbol{
						Name: n.Name, Kind: kind,
						Location: LSPLocation{Path: path, Line: pos.Line, Col: pos.Column},
					})
				}
			}
		}
		return out
	}
	return nil
}

func receiverTypeName(field *ast.Field) string {
	if field == nil {
		return ""
	}
	switch t := field.Type.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func declSignature(decl ast.Decl) string {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		var b strings.Builder
		b.WriteString("func ")
		if d.Recv != nil && len(d.Recv.List) > 0 {
			fmt.Fprintf(&b, "(%s) ", receiverTypeName(d.Recv.List[0]))
		}
		b.WriteString(d.Name.Name)
		b.WriteString("(")
		if d.Type.Params != nil {
			for i, p := range d.Type.Params.List {
				if i > 0 {
					b.WriteString(", ")
				}
				for j, n := range p.Names {
					if j > 0 {
						b.WriteString(", ")
					}
					b.WriteString(n.Name)
				}
				if len(p.Names) > 0 {
					b.WriteString(" ")
				}
				b.WriteString(exprToString(p.Type))
			}
		}
		b.WriteString(")")
		if d.Type.Results != nil {
			b.WriteString(" ")
			if len(d.Type.Results.List) > 1 || (len(d.Type.Results.List) == 1 && len(d.Type.Results.List[0].Names) > 0) {
				b.WriteString("(")
			}
			for i, r := range d.Type.Results.List {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(exprToString(r.Type))
			}
			if len(d.Type.Results.List) > 1 || (len(d.Type.Results.List) == 1 && len(d.Type.Results.List[0].Names) > 0) {
				b.WriteString(")")
			}
		}
		return b.String()
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok {
				return "type " + ts.Name.Name + " " + exprToString(ts.Type)
			}
		}
	}
	return ""
}

func exprToString(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return "*" + exprToString(x.X)
	case *ast.SelectorExpr:
		return exprToString(x.X) + "." + x.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprToString(x.Elt)
	case *ast.MapType:
		return "map[" + exprToString(x.Key) + "]" + exprToString(x.Value)
	case *ast.Ellipsis:
		return "..." + exprToString(x.Elt)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{...}"
	case *ast.FuncType:
		return "func(...)"
	}
	return "?"
}

func declDoc(decl ast.Decl) string {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Doc != nil {
			return strings.TrimSpace(d.Doc.Text())
		}
	case *ast.GenDecl:
		if d.Doc != nil {
			return strings.TrimSpace(d.Doc.Text())
		}
	}
	return ""
}

// errorsToDiagnostics unpacks parser errors. scanner.ErrorList stringifies
// to one line per underlying error separated by '\n', which is exactly
// the granularity diagnostics need.
func errorsToDiagnostics(err error) []LSPDiagnostic {
	var out []LSPDiagnostic
	for _, line := range strings.Split(err.Error(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, LSPDiagnostic{Severity: "error", Message: line})
	}
	return out
}

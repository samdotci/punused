package lib

import (
	"bytes"
	"context"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	lsp "github.com/sourcegraph/go-lsp"
	"golang.org/x/tools/go/analysis"
)

// Analyzer is the punused analyzer for finding unused exported Go symbols.
var Analyzer = &analysis.Analyzer{
	Name: "punused",
	Doc:  "finds unused exported Go symbols (functions, methods, variables, constants, fields, interfaces)",
	Run:  run,
}

var (
	// mu protects the gopls client initialization and caches
	mu          sync.Mutex
	initialized bool
	client      *GoplsClient
	workDir     string
	initErr     error
)

func run(pass *analysis.Pass) (interface{}, error) {
	mu.Lock()
	defer mu.Unlock()

	// Get the workspace directory from the file set
	if len(pass.Files) == 0 {
		return nil, nil
	}

	// Get the filename of the first file to determine workspace
	firstFile := pass.Fset.Position(pass.Files[0].Pos()).Filename
	currentWorkDir := findModuleRoot(filepath.Dir(firstFile))

	// Initialize or reinitialize the gopls client if needed
	if !initialized || workDir != currentWorkDir {
		if client != nil {
			client.Close()
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var err error
		client, err = newClient(ctx, currentWorkDir)
		if err != nil {
			initErr = fmt.Errorf("failed to initialize gopls client: %w", err)
			return nil, nil // Don't fail the whole analysis, just skip
		}

		workDir = currentWorkDir
		initialized = true
		initErr = nil
	}

	if initErr != nil {
		return nil, nil // Skip if initialization failed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Process each file in the pass
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename

		// Skip test files
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}

		// Make the filename relative to the workspace
		relPath, err := filepath.Rel(workDir, filename)
		if err != nil {
			continue
		}
		relPath = filepath.ToSlash(relPath)

		// Get symbols from gopls
		symbols, err := client.DocumentSymbol(ctx, relPath)
		if err != nil {
			continue // Skip files that fail
		}

		// Check each symbol
		var checkSymbol func(s *Symbol)
		checkSymbol = func(s *Symbol) {
			base := s.Name
			if s.Kind == lsp.SKMethod && strings.Contains(base, ".") {
				base = s.Name[strings.Index(s.Name, ".")+1:]
			}

			if !isExported(base) {
				return
			}

			refs, err := client.DocumentReferences(ctx, s.Location)
			if err != nil {
				return
			}

			var unused bool
			var testOnly bool
			if len(refs) == 0 {
				unused = true
			} else {
				testOnly = true
				for _, ref := range refs {
					if !strings.HasSuffix(string(ref.URI), "_test.go") {
						testOnly = false
						break
					}
				}
			}

			if unused || testOnly {
				// Map gopls location back to go/token position
				pos := findPosition(pass.Fset, filename, int(s.Location.Range.Start.Line), int(s.Location.Range.Start.Character))
				if pos == token.NoPos {
					return
				}

				kind := symbolKindToString(s.Kind)
				if testOnly {
					pass.Reportf(pos, "%s %s is used in test only (EU1001)", kind, s.Name)
				} else {
					pass.Reportf(pos, "%s %s is unused (EU1002)", kind, s.Name)
				}
			}

			for _, child := range s.Children {
				checkSymbol(child)
			}
		}

		for _, s := range symbols {
			checkSymbol(s)
		}
	}

	return nil, nil
}

// findModuleRoot finds the Go module root directory starting from dir
func findModuleRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir // Reached filesystem root
		}
		dir = parent
	}
}

// findPosition converts line/column to token.Pos
func findPosition(fset *token.FileSet, filename string, line, col int) token.Pos {
	var result token.Pos
	fset.Iterate(func(f *token.File) bool {
		if f.Name() == filename {
			if line < f.LineCount() {
				result = f.LineStart(line + 1) // Convert 0-based to 1-based
				if col > 0 {
					result += token.Pos(col)
				}
			}
			return false
		}
		return true
	})
	return result
}

// symbolKindToString converts lsp.SymbolKind to a string description
func symbolKindToString(kind interface{}) string {
	// lsp.SymbolKind values
	switch fmt.Sprintf("%v", kind) {
	case "1":
		return "file"
	case "2":
		return "module"
	case "3":
		return "namespace"
	case "4":
		return "package"
	case "5":
		return "class"
	case "6":
		return "method"
	case "7":
		return "property"
	case "8":
		return "field"
	case "9":
		return "constructor"
	case "10":
		return "enum"
	case "11":
		return "interface"
	case "12":
		return "function"
	case "13":
		return "variable"
	case "14":
		return "constant"
	case "15":
		return "string"
	case "16":
		return "number"
	case "17":
		return "boolean"
	case "18":
		return "array"
	case "19":
		return "object"
	case "20":
		return "key"
	case "21":
		return "null"
	case "22":
		return "enum member"
	case "23":
		return "struct"
	case "24":
		return "event"
	case "25":
		return "operator"
	case "26":
		return "type parameter"
	default:
		return "symbol"
	}
}

// RunForPlugin runs the punused analysis and returns results as a string buffer
// This is used by the plugin to integrate with golangci-lint
func RunForPlugin(ctx context.Context, workspaceDir string, pattern string) (*bytes.Buffer, error) {
	var buff bytes.Buffer
	err := Run(ctx, RunConfig{
		WorkspaceDir:    workspaceDir,
		FilenamePattern: pattern,
		Out:             &buff,
	})
	return &buff, err
}

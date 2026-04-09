package chunker

import (
	"context"
	"fmt"
	"slices"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/idelchi/godyl/pkg/path/file"
)

type langInfo struct {
	lang      *sitter.Language
	nodeTypes []string
}

var languages = map[string]langInfo{
	"go": {
		lang:      golang.GetLanguage(),
		nodeTypes: []string{"function_declaration", "method_declaration", "type_declaration"},
	},
	"py": {
		lang:      python.GetLanguage(),
		nodeTypes: []string{"function_definition", "class_definition"},
	},
	"ts": {
		lang: typescript.GetLanguage(),
		nodeTypes: []string{
			"function_declaration",
			"class_declaration",
			"method_definition",
			"interface_declaration",
			"type_alias_declaration",
			"enum_declaration",
		},
	},
	"tsx": {
		lang: tsx.GetLanguage(),
		nodeTypes: []string{
			"function_declaration",
			"class_declaration",
			"method_definition",
			"interface_declaration",
			"type_alias_declaration",
			"enum_declaration",
		},
	},
	"js": {
		lang:      javascript.GetLanguage(),
		nodeTypes: []string{"function_declaration", "class_declaration", "method_definition"},
	},
	"jsx": {
		lang:      javascript.GetLanguage(),
		nodeTypes: []string{"function_declaration", "class_declaration", "method_definition"},
	},
	"rs": {
		lang:      rust.GetLanguage(),
		nodeTypes: []string{"function_item", "impl_item", "struct_item", "enum_item", "trait_item"},
	},
	"java": {
		lang:      java.GetLanguage(),
		nodeTypes: []string{"class_declaration", "method_declaration", "interface_declaration"},
	},
	"c": {
		lang:      c.GetLanguage(),
		nodeTypes: []string{"function_definition", "struct_specifier", "type_definition"},
	},
	"h": {
		lang:      c.GetLanguage(),
		nodeTypes: []string{"function_definition", "struct_specifier", "type_definition"},
	},
	"cpp": {
		lang:      cpp.GetLanguage(),
		nodeTypes: []string{"function_definition", "class_specifier", "struct_specifier"},
	},
	"cc": {
		lang:      cpp.GetLanguage(),
		nodeTypes: []string{"function_definition", "class_specifier", "struct_specifier"},
	},
	"hpp": {
		lang:      cpp.GetLanguage(),
		nodeTypes: []string{"function_definition", "class_specifier", "struct_specifier"},
	},
}

type astChunker struct {
	maxTokens     int
	overlapTokens int
	lineFallback  *Chunker
}

func newastChunker(maxTokens, overlapTokens int, parent *Chunker) *astChunker {
	return &astChunker{
		maxTokens:     maxTokens,
		overlapTokens: overlapTokens,
		lineFallback:  parent,
	}
}

func (a *astChunker) SupportsExtension(path string) bool {
	ext := file.New(path).Extension()
	_, ok := languages[ext]

	return ok
}

type nodeBoundary struct {
	startByte uint32
	endByte   uint32
	startLine int
	endLine   int
}

func (a *astChunker) Chunk(ctx context.Context, path, content string) (Chunks, error) {
	ext := file.New(path).Extension()

	langInfo, ok := languages[ext]
	if !ok {
		return nil, fmt.Errorf("unsupported language: %s", ext)
	}

	contentBytes := []byte(content)
	parser := sitter.NewParser()
	parser.SetLanguage(langInfo.lang)

	tree, err := parser.ParseCtx(ctx, nil, contentBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	defer tree.Close()

	rootNode := tree.RootNode()

	var boundaries []nodeBoundary
	a.collectBoundaries(rootNode, langInfo.nodeTypes, &boundaries)

	if len(boundaries) == 0 {
		return a.lineFallback.chunkByLines(content), nil
	}

	return a.boundariesToChunks(contentBytes, boundaries)
}

func (a *astChunker) collectBoundaries(node *sitter.Node, types []string, out *[]nodeBoundary) {
	nodeType := node.Type()

	if slices.Contains(types, nodeType) {
		*out = append(*out, nodeBoundary{
			startByte: node.StartByte(),
			endByte:   node.EndByte(),
			startLine: int(node.StartPoint().Row) + 1,
			endLine:   int(node.EndPoint().Row) + 1,
		})

		return
	}

	for i := range int(node.ChildCount()) {
		child := node.Child(i)
		a.collectBoundaries(child, types, out)
	}
}

func (a *astChunker) boundariesToChunks(content []byte, boundaries []nodeBoundary) (Chunks, error) {
	var (
		chunks           Chunks
		currentContent   strings.Builder
		currentStartLine int
	)

	currentTokens := 0

	for i, boundary := range boundaries {
		nodeContent := string(content[boundary.startByte:boundary.endByte])
		nodeTokens := a.lineFallback.Estimate(nodeContent)

		if nodeTokens > a.maxTokens {
			if currentContent.Len() > 0 {
				chunks = append(chunks, Chunk{
					Index:     len(chunks),
					Content:   currentContent.String(),
					StartLine: currentStartLine,
					EndLine:   boundaries[i-1].endLine,
				})
				currentContent.Reset()

				currentTokens = 0
			}

			nodeChunks := a.lineFallback.chunkByLines(nodeContent)

			for _, nc := range nodeChunks {
				chunks = append(chunks, Chunk{
					Index:     len(chunks),
					Content:   nc.Content,
					StartLine: boundary.startLine + nc.StartLine - 1,
					EndLine:   boundary.startLine + nc.EndLine - 1,
				})
			}

			continue
		}

		if currentTokens+nodeTokens > a.maxTokens && currentContent.Len() > 0 {
			chunks = append(chunks, Chunk{
				Index:     len(chunks),
				Content:   currentContent.String(),
				StartLine: currentStartLine,
				EndLine:   boundaries[i-1].endLine,
			})
			currentContent.Reset()

			currentTokens = 0
		}

		if currentContent.Len() == 0 {
			currentStartLine = boundary.startLine
		} else {
			currentContent.WriteString("\n\n")
		}

		currentContent.WriteString(nodeContent)

		currentTokens += nodeTokens
	}

	if currentContent.Len() > 0 {
		chunks = append(chunks, Chunk{
			Index:     len(chunks),
			Content:   currentContent.String(),
			StartLine: currentStartLine,
			EndLine:   boundaries[len(boundaries)-1].endLine,
		})
	}

	return chunks, nil
}

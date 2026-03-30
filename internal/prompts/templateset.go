package prompts

import (
	"bytes"
	"fmt"
	"text/template"
	"text/template/parse"

	sprig "github.com/go-task/slim-sprig/v3"

	"github.com/idelchi/godyl/pkg/dag"
)

// TemplateSet manages named template partials, validates their dependency
// graph via DAG cycle detection, and renders a composed prompt from a
// designated entry point.
//
// Components (system prompt, agent prompt, mode prompt, autoloaded files,
// workspace instructions) are registered as named templates. The entry point
// template controls the composition order via {{ template "X" . }} for static
// references and {{ include "name" $ }} for dynamic (variable) references.
type TemplateSet struct {
	partials   map[string]string // name → raw template source
	entryPoint string            // which partial is the top-level render target
}

// NewTemplateSet creates a TemplateSet with the given entry point name.
// The entry point must be registered via Register before Render is called.
func NewTemplateSet(entryPoint string) *TemplateSet {
	return &TemplateSet{
		partials:   make(map[string]string),
		entryPoint: entryPoint,
	}
}

// Register adds a named template partial to the set.
// Duplicate names overwrite the previous registration.
func (ts *TemplateSet) Register(name, source string) {
	ts.partials[name] = source
}

// Validate parses all partials, extracts static {{ template "X" }} and
// {{ include "literal" }} references, builds a DAG, and returns an error
// if cycles or missing references exist.
//
// Dynamic {{ include .Variable $ }} references cannot be validated statically;
// they are caught at render time if the template does not exist.
func (ts *TemplateSet) Validate() error {
	refs := make(map[string][]string)

	for name, src := range ts.partials {
		// Use a dummy "include" func for parsing (the real one needs the assembled template).
		tmpl, err := template.New(name).Funcs(template.FuncMap{
			"include": func(string, any) (string, error) { return "", nil },
		}).Funcs(sprig.FuncMap()).Parse(src)
		if err != nil {
			return fmt.Errorf("parsing template %q: %w", name, err)
		}

		templateRefs := extractTemplateRefs(tmpl.Tree)
		includeRefs := extractIncludeRefs(tmpl.Tree)

		refs[name] = append(templateRefs, includeRefs...)
	}

	names := make([]string, 0, len(ts.partials))
	for name := range ts.partials {
		names = append(names, name)
	}

	// The DAG package uses "parents" terminology: if A includes B, then B is
	// a parent of A. Build validates no cycles and no missing dependencies.
	_, err := dag.Build(names, func(name string) []string {
		return refs[name]
	})

	return err
}

// Render validates the dependency graph, assembles the Go template with all
// partials as named templates, and executes the entry point with the given data.
func (ts *TemplateSet) Render(data any) (string, error) {
	if err := ts.Validate(); err != nil {
		return "", fmt.Errorf("template validation: %w", err)
	}

	root := template.New(ts.entryPoint).Option("missingkey=error")

	// The "include" function dynamically executes a named template.
	// Unlike {{ template "X" . }}, the name can come from a variable:
	//   {{ include .TemplateName $ }}
	root.Funcs(template.FuncMap{
		"include": func(name string, data any) (string, error) {
			var buf bytes.Buffer
			if err := root.ExecuteTemplate(&buf, name, data); err != nil {
				return "", err
			}

			return buf.String(), nil
		},
	})
	root.Funcs(sprig.FuncMap())

	for name, src := range ts.partials {
		var err error

		if name == ts.entryPoint {
			_, err = root.Parse(src)
		} else {
			_, err = root.New(name).Parse(src)
		}

		if err != nil {
			return "", fmt.Errorf("parsing template %q: %w", name, err)
		}
	}

	var buf bytes.Buffer
	if err := root.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// extractTemplateRefs walks a parse tree and finds {{ template "X" }} references.
func extractTemplateRefs(tree *parse.Tree) []string {
	var refs []string

	walkNodes(tree.Root, func(n parse.Node) {
		if tn, ok := n.(*parse.TemplateNode); ok {
			refs = append(refs, tn.Name)
		}
	})

	return refs
}

// extractIncludeRefs walks a parse tree and finds {{ include "literal" ... }} calls
// where the template name is a string constant (not a variable).
func extractIncludeRefs(tree *parse.Tree) []string {
	var refs []string

	walkNodes(tree.Root, func(n parse.Node) {
		an, ok := n.(*parse.ActionNode)
		if !ok || an.Pipe == nil {
			return
		}

		for _, cmd := range an.Pipe.Cmds {
			if len(cmd.Args) < 2 {
				continue
			}

			ident, ok := cmd.Args[0].(*parse.IdentifierNode)
			if !ok || ident.Ident != "include" {
				continue
			}

			str, ok := cmd.Args[1].(*parse.StringNode)
			if ok {
				refs = append(refs, str.Text)
			}
		}
	})

	return refs
}

// walkNodes recursively visits all nodes in a parse tree.
func walkNodes(node parse.Node, fn func(parse.Node)) {
	fn(node)

	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}

		for _, child := range n.Nodes {
			walkNodes(child, fn)
		}
	case *parse.IfNode:
		walkNodes(n.List, fn)

		if n.ElseList != nil {
			walkNodes(n.ElseList, fn)
		}
	case *parse.RangeNode:
		walkNodes(n.List, fn)

		if n.ElseList != nil {
			walkNodes(n.ElseList, fn)
		}
	case *parse.WithNode:
		walkNodes(n.List, fn)

		if n.ElseList != nil {
			walkNodes(n.ElseList, fn)
		}
	}
}

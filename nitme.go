// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nitme

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var analyzer = &analysis.Analyzer{
	Name: "nitme",
	Doc:  "This analyzer catches nits before your reviewer does.",
	Run:  run,
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
	},
}

func run(pass *analysis.Pass) (interface{}, error) {
	// inspect's Analyzer produces an Inspector, which allows easy traversal of the ast
	ins := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// The Inspector requires us to specify the nodes we are interested in.
	// We only care about AssignStmt nodes.
	nodeFilter := []ast.Node{
		(*ast.AssignStmt)(nil),
	}

	// We call the Inspector, telling it what nodes we are interested in,
	// as well as what function we want to run on each interesting node.
	ins.Preorder(nodeFilter, func(n ast.Node) {
		assignment := n.(*ast.AssignStmt)

		// If the assignment's RHS is not a composite, skip it.
		composite, ok := assignment.Rhs[0].(*ast.CompositeLit)
		if !ok {
			return
		}

		// If the composite is not empty, skip it.
		if composite.Elts != nil {
			return
		}

		// If the composite is not an array, skip it.
		arrayType, ok := composite.Type.(*ast.ArrayType)
		if !ok {
			return
		}

		ident := assignment.Lhs[0].(*ast.Ident)
		elt := arrayType.Elt.(*ast.Ident)
		report(pass, n, ident.Name, elt.Name)
	})

	return nil, nil
}

func report(pass *analysis.Pass, n ast.Node, sliceName, eltName string) {
	pass.Report(analysis.Diagnostic{
		Pos:     n.Pos(),
		Message: "incorrect empty slice declaration",
		SuggestedFixes: []analysis.SuggestedFix{{
			Message: fmt.Sprintf("use var"),
			TextEdits: []analysis.TextEdit{{
				Pos:     n.Pos(),
				End:     n.End(),
				NewText: []byte(fmt.Sprintf("var %s []%s", sliceName, eltName)),
			}},
		}},
	})
}

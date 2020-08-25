# Tutorial: Writing a Go Static Analyzer

Your reviewer came back with the same comments again: identifiers should use camelCase, not snake\_case; exported types should be documented; [empty slices should be declared using `var`](https://gist.github.com/adamveld12/c0d9f0d5f0e1fba1e551#declaring-empty-slices).

You try to be meticulous, but sooner or later the same mistakes always seem to seep through. If only there were a way to harness your computer's unfettering supply of discipline to prevent these errors from ever reaching your reviewer's eyes...

Despair no more. In the following tutorial, you will learn how to write a Go static analyzer. Go provides a nice framework for writing and testing static analysis tools, so this will be rather straightforward. Take a new directory, and let's begin!

The first step is to describe the errors we want to catch:

`testdata/src/tests/test.go`
```
package nitme

func test() {
	var correct []int
	nonEmpty := []int{0}
	incorrect := []int{}                   // want "incorrect empty slice declaration"
	_, _, _ = correct, incorrect, nonEmpty // to prevent compiler complaints about unused names
}
```

As you have astutely noticed, this is just Go code. Expected error reports are written in a comment on the line where an error occurs. These comments *must* take the form `want "[...]"`. Note that not putting a comment on a line also indicates an expectation: `var correct []int` is a correct empty slice declaration, so we expect that nothing will be reported.

We can now start thinking about how we are going to detect incorrect empty slice declarations. For this purpose, we will use the `gotype` command along with the `-ast` flag to reveal the structure of the code. The `-ast` flag asks `gotype` to output the **A**bstract **S**yntax **T**ree (AST) of the code. This is simply a tree-like representation of the code that is easy to manipulate programmatically. Since I like frequently used commands to have names that are short and obvious, I use an alias: `alias astdump='gotype -ast'`.

Running `astdump testdata/src/tests/test.go` produces quite a bit of output. (Feel free to scrutinize the output if this is the first time you dump a program's AST.) First, we need to identify what each declaration maps to in the AST. This is fairly easy, because the AST closely follows the code's structure.

`var correct []int` maps to:
```
27  .  .  .  .  .  0: *ast.DeclStmt {

[...]

57  .  .  .  .  .  }
```

I've omitted most of the subtree because all we need to know about this piece of code is that it is an `*ast.DeclStmt`. As you'll see next, we do not need to bother ourselves with `DeclStmt`s.

`nonEmpty := []int{0}` maps to:
```
58  .  .  .  .  .  1: *ast.AssignStmt {
59  .  .  .  .  .  .  Lhs: []ast.Expr (len = 1) {
60  .  .  .  .  .  .  .  0: *ast.Ident {

[...]

62  .  .  .  .  .  .  .  .  Name: "nonEmpty"

[...]

68  .  .  .  .  .  .  .  }
69  .  .  .  .  .  .  }

[...]

72  .  .  .  .  .  .  Rhs: []ast.Expr (len = 1) {
73  .  .  .  .  .  .  .  0: *ast.CompositeLit {
74  .  .  .  .  .  .  .  .  Type: *ast.ArrayType {

[...]

82  .  .  .  .  .  .  .  .  Elts: []ast.Expr (len = 1) {

[...]

88  .  .  .  .  .  .  .  .  }

[...]

91  .  .  .  .  .  .  .  }
92  .  .  .  .  .  .  }
93  .  .  .  .  .  }
```

Once again, I've omitted the irrelevant parts. This subtree describes an assignment statement that assigns a **composite literal** containing 1 element to a variable named `nonEmpty`.
 
Finally, `incorrect := []int{}` maps to:
```
94  .  .  .  .  .  2: *ast.AssignStmt {
95  .  .  .  .  .  .  Lhs: []ast.Expr (len = 1) {
96  .  .  .  .  .  .  .  0: *ast.Ident {

[...]

98  .  .  .  .  .  .  .  .  Name: "incorrect"

[...]

104  .  .  .  .  .  .  .  }
105  .  .  .  .  .  .  }

[...]

108  .  .  .  .  .  .  Rhs: []ast.Expr (len = 1) {
109  .  .  .  .  .  .  .  0: *ast.CompositeLit {
110  .  .  .  .  .  .  .  .  Type: *ast.ArrayType {

[...]

120  .  .  .  .  .  .  .  }
121  .  .  .  .  .  .  }
122  .  .  .  .  .  }
```

Here's a useful observation: when assigning an empty slice to a variable, the **composite literal** has no `Elts` field! (Or rather, its `Elts` field is nil, so `astdump` does not print it.)

Before we can apply this crucial insight, let's write a test that will let us know when our analyzer is working. This is essentially boilerplate:
`nitme_test.go`
```
package nitme

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, analyzer, "tests")
}
```

(`analyzer` has not been defined yet, so this code should not compile. We will define `analyzer` very soon).

The test is simple to write because `analysistest` takes care of most of the details (e.g., finding the test files, running our analyzer on them, and verifying that the expected reports are produced).

The code for our analyzer is simple, and I've made a special effort to sprinkle it with helpful comments, so I'll just let you dive in:
`nitme.go`
```
package nitme

import (
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
		assignment, _ := n.(*ast.AssignStmt)

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
		_, isArray := composite.Type.(*ast.ArrayType)
		if !isArray {
			return
		}

		// At this point, we know the assignment is declaring an empty slice,
		// so we report it.
		pass.Reportf(assignment.Pos(), "incorrect empty slice declaration")
	})

	return nil, nil
}
```

And... that's it! Now we only need to remember to run our analyzer. Build it, put it somewhere on your PATH (I would recommend $GOPATH/bin), and invoke it using `nitme <file>`.

## Automatically fixing errors

Now you might be thinking, "Cool, now my computer can yell at me, instead of my reviewer" (hopefully, your reviewers aren't actually yelling at you). But wouldn't it be great if no one (or thing) yelled at you at all? Indeed, changing `mySlice := []int{}` to `var mySlice []int` isn't exactly worthy of anyone's time, so let's have the computer do it for us.

Once again, it's a good idea to start with the end in mind: let's update our tests to reflect our new requirement. To do this, we just need to copy our `testdata/src/tests/tests.go` file to a reference file named `testdata/src/tests/test.go.golden`, and make the change we want our analyzer to perform automatically.

Before (in `tests.go`):
```
	incorrect := []int{}                   // want "incorrect empty slice declaration"
```

After (in `tests.go.golden`):
```
	var incorrect []int                    // want "incorrect empty slice declaration"
```

We also need to make a small change in `nitme_test.go` to let `analysistest` know that we want to verify that our fix is applied correctly: `analysistest.Run(...)` becomes `analysistest.RunWithSuggestedFixes(...)`.

We can now implement the feature. When we report an incorrect empty slice declaration, we just need to provide a suggested fix:
```
[...]

			// At this point, we know the assignment is declaring an empty slice,
			// so we report it.
			report(pass, n, ident.Name, elt.Name)
		}
	})

	return nil, nil
}

func report(pass *analysis.Pass, n ast.Node, sliceName, eltName string) {
	pass.Report(analysis.Diagnostic{
		Pos:     n.Pos(),
		Message: "incorrect empty slice declaration",
		SuggestedFixes: []analysis.SuggestedFix{{
			Message: "use var",
			TextEdits: []analysis.TextEdit{{
				Pos:     n.Pos(),
				End:     n.End(),
				NewText: []byte(fmt.Sprintf("var %s []%s", sliceName, eltName)),
			}},
		}},
	})
}
```

Now when you call `nitme`, you just need to tell it to apply the fixes: `nitme -fix <file>`. Thanks, static analysis.

## Parting words

I hope you've enjoyed working through this tutorial. More importantly, I hope you now have a slightly better understanding of how to get started on writing a static analyzer in Go. I encourage you to take a look at the `golang.org/x/tools/go/analysis` package. In particular, the `analysis` package contains a lot of examples of analyzers in its `passes` directory.


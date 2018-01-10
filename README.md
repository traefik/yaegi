# Go interpreter

A Go interpreter in go

## Developpement plan

### Step 1: a symbolic interpreter

The fisrt step consists to create a *symbolic* interpreter, generic, architecture independent, with a minimal execution layer on top of the abstract syntax tree (AST).

The lexical and syntaxic analysis are to be reused from Go toolchain code (up to AST production).

The AST (abstract syntax tree) will be extended to permit in place execution through extending the AST to a control flow graph (CFG), using a technique developped for bip (private project), and allowing high speed interpretation on top of AST.

The result of first step is a fully workable and portable Go interpreter with all features present in the language, but with a limited level of performances.

The starting point is the bip interpreter (Marc's private project) in C, ported to Go.

### Step 2: an optimizing tracing JIT compiler

The second step consists to add a dynamic code generator, thus extending the interperter with a Just-In-Time compiler. 

The dynamic code generator converts parts of the intermediate representation (IR), here a specialy annotated AST, directly to machine code in executable memory pages. It is expected to reuse parts of the Go assembler for the machine code generation).

One idea is to use traces from the interpreter to replace hot paths (i.e. intensive loops) by snippets of stream-lined native code. The code generator doesn't need to be complete to start gaining performance improvemeents, as only parts where code generator is guaranteed to works will be applied, and the symbolic execution level serves as a fallback.

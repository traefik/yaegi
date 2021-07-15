package interp

// FileResult is the result of evaluating a file-like source.
type FileResult struct {
	Statements []FileStatementResult
}

// FileStatementResult is the result of a top-level statement in a file-like source.
type FileStatementResult interface {
	isFileStatementResult()
}

func (*PackageImportResult) isFileStatementResult()       {}
func (*FunctionDeclarationResult) isFileStatementResult() {}
func (*TypeDeclarationResult) isFileStatementResult()     {}

// PackageImportResult is the result of a package import statement.
type PackageImportResult struct {
	Name, Path string
}

// FunctionDeclarationResult is the result of a top-level function declaration statement.
type FunctionDeclarationResult struct {
	Name string
}

// TypeDeclarationResult is the result of a top-level type declaration statement.
type TypeDeclarationResult struct {
	Name string
}

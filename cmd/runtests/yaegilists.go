package main

// Lists taken from yaegi's interp tests (tests/yaegi/api/master/interp/):
//
//   - interp_file_test.go (TestFile): the only skip list is testsToSkipGo121,
//     applied on go1.21 only, so TestFile runs every program on the current
//     toolchain. The runner does the same.
//   - interp_consistent_test.go (TestInterpConsistencyBuild): programs not
//     compared with `go build` output, with the reason. Informational here: the
//     reason is reported in the TSV note column.
//   - interp_consistent_test.go (TestInterpErrorConsistency): for some Error:
//     programs, the message gc gives (expectedExec). The VM type-checks with
//     go/types, whose messages are gc's, so a match on that message is counted
//     as PASS_ERR_GC.
//
// Additions from fork branches and upstream PRs imported into tests/yaegi are
// marked with their origin.

// consistencyReason is the skip list of TestInterpConsistencyBuild.
var consistencyReason = map[string]string{
	"assign11.go":              "expect error",
	"assign12.go":              "expect error",
	"assign15.go":              "expect error",
	"assign19.go":              "expect error",
	"bad0.go":                  "expect error",
	"break0.go":                "expect error",
	"cont3.go":                 "expect error",
	"const9.go":                "expect error",
	"export1.go":               "non-main package",
	"export0.go":               "non-main package",
	"for7.go":                  "expect error",
	"fun21.go":                 "expect error",
	"fun22.go":                 "expect error",
	"fun23.go":                 "expect error",
	"fun24.go":                 "expect error",
	"fun25.go":                 "expect error",
	"gen7.go":                  "expect error",
	"gen8.go":                  "expect error",
	"gen9.go":                  "expect error",
	"if2.go":                   "expect error",
	"import6.go":               "expect error",
	"init1.go":                 "expect error",
	"io0.go":                   "use random number",
	"issue-1093.go":            "expect error",
	"issue-1276.go":            "expect error",
	"issue-1330.go":            "expect error",
	"op1.go":                   "expect error",
	"op7.go":                   "expect error",
	"op9.go":                   "expect error",
	"bltn0.go":                 "expect error",
	"method16.go":              "private struct field",
	"method39.go":              "expect error",
	"switch8.go":               "expect error",
	"switch9.go":               "expect error",
	"switch13.go":              "expect error",
	"switch19.go":              "expect error",
	"time0.go":                 "display time (similar to random number)",
	"factor.go":                "bench",
	"fib.go":                   "bench",
	"issue-1618.go":            "bench (infinite running)",
	"type5.go":                 "reflect cannot create named types",
	"type6.go":                 "reflect cannot create named types",
	"redeclaration0.go":        "expect error",
	"redeclaration1.go":        "expect error",
	"redeclaration2.go":        "expect error",
	"redeclaration3.go":        "expect error",
	"redeclaration4.go":        "expect error",
	"redeclaration5.go":        "expect error",
	"redeclaration-global0.go": "expect error",
	"redeclaration-global1.go": "expect error",
	"redeclaration-global2.go": "expect error",
	"redeclaration-global3.go": "expect error",
	"redeclaration-global4.go": "expect error",
	"redeclaration-global5.go": "expect error",
	"redeclaration-global6.go": "expect error",
	"redeclaration-global7.go": "expect error",
	"panic0.go":                "expect error",
	"pkgname0.go":              "has deps",
	"pkgname1.go":              "expect error",
	"pkgname2.go":              "has deps",
	"ipp_as_key.go":            "has deps",
	"restricted0.go":           "expect error",
	"restricted1.go":           "expect error",
	"restricted2.go":           "expect error",
	"restricted3.go":           "expect error",
	"server6.go":               "syntax parsing",
	"server5.go":               "syntax parsing",
	"server4.go":               "syntax parsing",
	"server3.go":               "syntax parsing",
	"server2.go":               "syntax parsing",
	"server1a.go":              "syntax parsing",
	"server1.go":               "syntax parsing",
	"server0.go":               "syntax parsing",
	"server.go":                "syntax parsing",
	"range9.go":                "expect error",
	"unsafe6.go":               "needs go.mod to be 1.17",
	"unsafe7.go":               "needs go.mod to be 1.17",
	"type24.go":                "expect error",
	"type27.go":                "expect error",
	"type28.go":                "expect error",
	"type29.go":                "expect error",
	"type30.go":                "expect error",
	"type31.go":                "expect error",
	"type32.go":                "expect error",
	"type33.go":                "expect error",

	// fork:ai/fix-upstream-1437
	"issue-1437e.go": "expect error",
	// fork:ai/fix-upstream-1674
	"clear1.go": "expect error",
	"max1.go":   "expect error",
	"max2.go":   "expect error",
	"min1.go":   "expect error",
	// upstream-pr:1729
	"for20.go": "expect error",
	"for21.go": "expect error",
	"if10.go":  "expect error",
	"if8.go":   "expect error",
	"if9.go":   "expect error",
	// upstream-pr:1730
	"issue-1593.go": "expect error",
	// upstream-pr:1732
	"upstream-1196.go": "expect error",
	"upstream-633.go":  "interface-element containers do not marshal (documented)",
	// upstream-pr:1737
	"gen17.go": "has deps",
	"gen19.go": "expect error",
	"gen27.go": "has deps",
}

// gcError is expectedExec of TestInterpErrorConsistency: the message gc
// reports for an Error: program whose yaegi message differs.
var gcError = map[string]string{
	"assign11.go":   "6:12: assignment mismatch: 3 variables but fmt.Println returns 2 values",
	"assign12.go":   "6:13: assignment mismatch: 3 variables but fmt.Println returns 2 values",
	"bad0.go":       "1:1: expected 'package', found println",
	"break0.go":     "15:11: invalid break label OuterLoop",
	"cont3.go":      "15:14: invalid continue label OuterLoop",
	"const9.go":     "5:2: initialization",
	"if2.go":        "7:5: non-boolean condition in if statement",
	"for7.go":       "4:14: non-boolean condition in for statement",
	"fun21.go":      "4:2: not enough return values",
	"fun22.go":      "6:2: not enough arguments in call to time.Date",
	"fun23.go":      "3:24: too many return values",
	"issue-1093.go": `9:6: cannot use "a" + b() (value of type string)`,
	"op1.go":        "5:7: 1.3 (untyped float constant) truncated to int",
	"bltn0.go":      "4:7: println (built-in) must be called",
	"import6.go":    "import cycle not allowed",
	"switch8.go":    "5:2: fallthrough statement out of place",
	"switch9.go":    "fallthrough",
	"switch13.go":   "is not a type",
	"switch19.go":   "37:7: duplicate case Bir in type switch",

	// fork:ai/fix-upstream-1437
	"issue-1437e.go": "3:11: undefined: baz",
	// upstream-pr:1729
	"if8.go":   "4:5: non-boolean condition in if statement",
	"if9.go":   "4:5: non-boolean condition in if statement",
	"if10.go":  "4:13: non-boolean condition in if statement",
	"for20.go": "4:6: non-boolean condition in for statement",
	"for21.go": "4:14: non-boolean condition in for statement",
}

// apiOnly lists programs that import yaegi's own interp package (they test
// yaegi from inside yaegi): not runnable by the VM by design.
var apiOnly = map[string]bool{
	"eval0.go":     true,
	"inception.go": true,
}

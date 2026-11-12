package main

import (
	"go/token"
	"go/types"
	"testing"
)

// TestMentionsCopiedInternal checks the rule that copies
// encoding/json/jsontext with Go 1.27: encoding/json/v2 calls jsontext with
// types of the copied encoding/json/internal/jsonopts, through an alias
// (json.Options = jsonopts.Options).
func TestMentionsCopiedInternal(t *testing.T) {
	opts := types.NewPackage("encoding/json/internal/jsonopts", "jsonopts")
	iface := types.NewInterfaceType(nil, nil)
	named := types.NewNamed(types.NewTypeName(token.NoPos, opts, "Options", nil), iface, nil)
	v2 := types.NewPackage("encoding/json/v2", "json")
	alias := types.NewAlias(types.NewTypeName(token.NoPos, v2, "Options", nil), named)
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, v2, "opts", types.NewSlice(alias))), nil, true)

	g := &gen{pkgs: map[string]*pinfo{opts.Path(): {kind: kNative}}}
	if g.mentionsCopiedInternal(sig) {
		t.Error("a native internal package is reported")
	}
	g.pkgs[opts.Path()].kind = kCopy
	if !g.mentionsCopiedInternal(sig) {
		t.Error("a copied internal package behind an alias is not reported")
	}
	public := types.NewNamed(types.NewTypeName(token.NoPos, v2, "Marshalers", nil), iface, nil)
	g.pkgs[v2.Path()] = &pinfo{kind: kConvert}
	if g.mentionsCopiedInternal(types.NewPointer(public)) {
		t.Error("a package that is not internal is reported")
	}
}

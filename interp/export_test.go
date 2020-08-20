package interp

func (interp *Interpreter) Scopes() map[string]map[string]struct{} {
	scopes := make(map[string]map[string]struct{})
	for k, v := range interp.scopes {
		syms := make(map[string]struct{})
		for kk := range v.sym {
			syms[kk] = struct{}{}
		}
		scopes[k] = syms
	}
	return scopes
}

func (interp *Interpreter) Packages() map[string]string {
	return interp.pkgNames
}

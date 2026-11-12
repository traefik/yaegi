package vm

// GenericImplemented reports whether the VM runs the instances of the
// generic function name ("pkgpath.Func") for the types of a program: the
// AOT copy registered a hook (RegisterGeneric), or genericImpls has it.
func GenericImplemented(name string) bool {
	regMu.RLock()
	defer regMu.RUnlock()
	_, hook := aotGenerics[name]
	_, impl := genericImpls[name]
	return hook || impl
}

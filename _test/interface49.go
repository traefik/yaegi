package main

type Descriptor interface {
	ParentFile() FileDescriptor
}

type FileDescriptor interface {
	Enums() EnumDescriptors
	Services() ServiceDescriptors
}

type EnumDescriptors interface {
	Get(i int) EnumDescriptor
}

type EnumDescriptor interface {
	Values() EnumValueDescriptors
}

type EnumValueDescriptors interface {
	Get(i int) EnumValueDescriptor
}

type EnumValueDescriptor interface {
	Descriptor
}

type ServiceDescriptors interface {
	Get(i int) ServiceDescriptor
}

type ServiceDescriptor interface {
	Descriptor
	isServiceDescriptor
}

type isServiceDescriptor interface{ ProtoType(ServiceDescriptor) }

func main() {
	var d Descriptor
	println(d == nil)
}

// Output:
// true

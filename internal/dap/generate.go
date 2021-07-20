package dap

//go:generate go run ../cmd/genschema --dap-mode --name dap --path schema.go --patch patch.json --url https://microsoft.github.io/debug-adapter-protocol/debugAdapterProtocol.json
//go:generate go run ../cmd/gendap --name dap --path types.go --patch patch.json --url https://microsoft.github.io/debug-adapter-protocol/debugAdapterProtocol.json

module modes

go 1.26.0

// The versions are the ones of the go.mod of yaegi v2, so that the AOT copies
// of aot/ match (-aot uses a copy only for the version it was made from).
require (
	github.com/go-playground/validator/v10 v10.30.2
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/go-cmp v0.7.0
	github.com/json-iterator/go v1.1.13-0.20220915233716-71ac16282d12
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

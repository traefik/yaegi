module capab

go 1.26.0

require (
	example.com/asmlib v0.0.0
	example.com/minicfg v0.0.0
	github.com/Masterminds/semver/v3 v3.4.0
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/fatih/structs v1.1.0
	github.com/gabriel-vasile/mimetype v1.4.13
	github.com/go-playground/locales v0.14.1
	github.com/go-playground/universal-translator v0.18.1
	github.com/go-playground/validator/v10 v10.30.2
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/hashicorp/golang-lru v1.0.2
	github.com/json-iterator/go v1.1.13-0.20220915233716-71ac16282d12
	github.com/klauspost/compress v1.18.0
	github.com/leodido/go-urn v1.4.0
	github.com/mailru/easyjson v0.7.7
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/cast v1.10.0
	github.com/tidwall/gjson v1.17.0
	github.com/tidwall/pretty v1.2.1
	golang.org/x/crypto v0.57.0
	golang.org/x/net v0.60.0
	golang.org/x/sys v0.48.0
	golang.org/x/text v0.42.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/josharian/intern v1.0.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
)

replace example.com/minicfg => ../minilib

replace example.com/asmlib => ../asmlib

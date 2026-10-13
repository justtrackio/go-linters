package linters

import (
	"go/ast"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("cfgsecret", NewCfgSecret)
}

// CfgSecretPlugin reports credential-like configuration fields without redaction.
type CfgSecretPlugin struct{}

func (*CfgSecretPlugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func (*CfgSecretPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "cfgsecret",
		Doc:  "reports credential-like cfg fields missing the secret option",
		Run:  runCfgSecret,
	}}, nil
}

func NewCfgSecret(_ any) (register.LinterPlugin, error) {
	return &CfgSecretPlugin{}, nil
}

func runCfgSecret(pass *analysis.Pass) (any, error) {
	var inspectStruct func(*ast.StructType)
	inspectStruct = func(st *ast.StructType) {
		for _, field := range st.Fields.List {
			// Gosoline flattens embedded fields and ignores their tag options.
			embedded := len(field.Names) == 0
			exported := embedded

			for _, name := range field.Names {
				exported = exported || name.IsExported()
			}

			if !exported {
				continue
			}

			key, tagged, secret := cfgSecretTag(field)
			if !embedded && tagged && secret {
				// A secret parent redacts the complete inline subtree.
				continue
			}

			if !embedded && !tagged {
				// Untagged named fields are not part of Gosoline's cfg mapping.
				continue
			}

			if !embedded && tagged {
				for _, name := range field.Names {
					if name.IsExported() && (cfgSecretName(name.Name) || cfgSecretName(key)) {
						pass.Reportf(name.Pos(), "cfgsecret: config field %s may contain credentials; add the secret option to its cfg tag", name.Name)
					}
				}
			}

			ast.Inspect(field.Type, func(node ast.Node) bool {
				if child, ok := node.(*ast.StructType); ok {
					inspectStruct(child)
					return false
				}

				return true
			})
		}
	}

	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			if st, ok := node.(*ast.StructType); ok {
				inspectStruct(st)
				return false
			}

			return true
		})
	}

	return nil, nil
}

func cfgSecretTag(field *ast.Field) (key string, tagged, secret bool) {
	var err error
	var tag string

	if field.Tag == nil {
		return "", false, false
	}

	if tag, err = strconv.Unquote(field.Tag.Value); err != nil {
		return "", false, false
	}

	value, tagged := reflect.StructTag(tag).Lookup("cfg")
	if !tagged {
		return "", false, false
	}

	parts := strings.Split(value, ",")
	key = strings.TrimSpace(parts[0])

	for _, option := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(option), "secret") {
			secret = true
		}
	}

	return key, true, secret
}

func cfgSecretName(name string) bool {
	// Config selectors may contain a path; classify its final component only.
	if index := strings.LastIndexByte(name, '.'); index >= 0 {
		name = name[index+1:]
	}

	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}

		return -1
	}, name)

	for _, suffix := range []string{"password", "passwd", "passphrase", "secret", "privatekey", "apikey", "secretkey", "secretaccesskey", "signingkey"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}

	switch name {
	case "token", "authtoken", "accesstoken", "refreshtoken", "sessiontoken", "bearertoken", "apitoken", "clienttoken", "idtoken", "jwttoken", "personalaccesstoken", "authorization", "proxyauthorization", "authkeys", "apiauthkeys", "basicauthusers", "apiauthbasicusers":
		return true
	default:
		return false
	}
}

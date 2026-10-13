package cfgsecret

type Settings struct {
	Password                  string   `cfg:"password"`              // want "cfgsecret: config field Password"
	ClientSecret              string   `cfg:"client_secret"`         // want "cfgsecret: config field ClientSecret"
	SessionToken              string   `cfg:"session_token"`         // want "cfgsecret: config field SessionToken"
	SigningKey                []byte   `cfg:"signing_key"`           // want "cfgsecret: config field SigningKey"
	PrivateKey                string   `cfg:"privateKey"`            // want "cfgsecret: config field PrivateKey"
	SecretKey                 string   `cfg:"secret_key"`            // want "cfgsecret: config field SecretKey"
	Opaque                    string   `cfg:"API_KEY"`               // want "cfgsecret: config field Opaque"
	AuthKeys                  []string `cfg:"api_auth_keys"`         // want "cfgsecret: config field AuthKeys"
	Users                     []string `cfg:"api_auth_basic_users"`  // want "cfgsecret: config field Users"
	Header                    string   `cfg:"headers.Authorization"` // want "cfgsecret: config field Header"
	SecretNamedButNotRedacted string   `cfg:"secret"`                // want "cfgsecret: config field SecretNamedButNotRedacted"
	Misplaced                 string   `cfg:"secret,password"`       // want "cfgsecret: config field Misplaced"
	Typo                      string   `cfg:"access_token,secrets"`  // want "cfgsecret: config field Typo"
	PasswordA, RootPassword   string   `cfg:"opaque"`                // want "cfgsecret: config field RootPassword"
	private, UserPassword     string   `cfg:"opaque"`                // want "cfgsecret: config field UserPassword"

	SafePassword      string `cfg:"password,secret"`
	SecretAccessKey   string `cfg:"secret_access_key, SeCrEt ,nodecode,nocast"`
	EmptyKeyToken     string `cfg:",secret"`
	QuotedSecret      string "cfg:\"api_key,secret\""
	NotCfgPassword    string `json:"password"`
	UntaggedPassword  string
	privatePassword   string            `cfg:"password"`
	PublicKey         string            `cfg:"public_key"`
	AccessKeyID       string            `cfg:"access_key_id"`
	SecretName        string            `cfg:"secret_name"`
	SecretARN         string            `cfg:"secret_arn"`
	SecretFile        string            `cfg:"secret_file"`
	TokenExpiry       int               `cfg:"token_expiry"`
	TokenURL          string            `cfg:"token_url"`
	TokenCount        int               `cfg:"token_count"`
	PageToken         string            `cfg:"page_token"`
	ContinuationToken string            `cfg:"continuation_token"`
	Headers           map[string]string `cfg:"headers"`
	PublicValue       string            `cfg:"secret.public_value"`

	Hidden struct {
		Password string `cfg:"password"`
	} `cfg:"hidden,secret"`
	HiddenRows []struct {
		Token string `cfg:"token"`
	} `cfg:"rows,secret"`
	HiddenMap map[string]struct {
		ApiKey string `cfg:"api_key"`
	} `cfg:"clients,secret"`
	Ignored struct {
		Password string `cfg:"password"`
	}
	Nested *struct {
		RefreshToken string `cfg:"refresh_token"` // want "cfgsecret: config field RefreshToken"
	} `cfg:"nested"`
	InlineRows []struct {
		Value string `cfg:"password"` // want "cfgsecret: config field Value"
	} `cfg:"inline_rows"`
	structWithSecret `cfg:",secret"`
}

// Named types are checked independently: one secret parent does not cover reuse.
type structWithSecret struct {
	ApiKey string `cfg:"api_key"` // want "cfgsecret: config field ApiKey"
}

type OtherSettings struct {
	Private structWithSecret `cfg:"private,secret"`
	Public  structWithSecret `cfg:"public"`
}

var local = struct {
	DBPassword string `cfg:"db_password"` // want "cfgsecret: config field DBPassword"
}{}

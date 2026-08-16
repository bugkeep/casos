package casdoor

import (
	_ "embed"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/casosorg/casos/conf"
)

//go:embed token_jwt_key.pem
var JwtPublicKey string

// InitCasdoorConfig initializes the Casdoor SDK when Casdoor is configured, and
// records the outcome so the rest of the process can fall back to the built-in
// password sign-in. An empty casdoorEndpoint means "no Casdoor", which is the
// default for a fresh installation.
func InitCasdoorConfig() {
	casdoorEndpoint := conf.GetConfigString("casdoorEndpoint")
	if casdoorEndpoint == "" {
		conf.SetCasdoorAvailable(false)
		return
	}

	casdoorsdk.InitConfig(
		casdoorEndpoint,
		conf.GetConfigString("clientId"),
		conf.GetConfigString("clientSecret"),
		JwtPublicKey,
		conf.GetConfigString("casdoorOrganization"),
		conf.GetConfigString("casdoorApplication"),
	)
	conf.SetCasdoorAvailable(true)
}

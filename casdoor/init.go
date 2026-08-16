package casdoor

import (
	_ "embed"
)

//go:embed token_jwt_key.pem
var JwtPublicKey string

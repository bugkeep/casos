//go:build embed

package conf

import (
	_ "embed"

	beegoconfig "github.com/beego/beego/config"
)

const standaloneBuild = true

//go:embed app.conf
var standaloneAppConfig []byte

var standaloneDefaults beegoconfig.Configer

func init() {
	config, err := beegoconfig.NewConfigData("ini", standaloneAppConfig)
	if err != nil {
		panic(err)
	}
	standaloneDefaults = config
}

func standaloneConfigValue(key string) string {
	if standaloneDefaults == nil {
		return ""
	}
	return standaloneDefaults.String(key)
}

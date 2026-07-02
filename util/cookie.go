package util

import (
	"encoding/json"

	"github.com/beego/beego/context"
	"github.com/casosorg/casos/conf"
)

func AppendWebConfigCookie(ctx *context.Context) error {
	webConfig := conf.GetWebConfig()
	jsonWebConfig, err := json.Marshal(webConfig)
	if err != nil {
		return err
	}
	ctx.SetCookie("jsonWebConfig", string(jsonWebConfig))
	return nil
}

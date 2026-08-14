//go:build !embed

package conf

const standaloneBuild = false

func standaloneConfigValue(string) string {
	return ""
}

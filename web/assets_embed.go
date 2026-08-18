//go:build embed

package webassets

import (
	"embed"
	"io/fs"
)

// Standalone binaries ship the shadcn frontend from web2/build, so this tree is
// deliberately not embedded: doing so would carry a second copy of the whole UI
// in every release and force `-tags embed` builds to run `yarn build` here as
// well, for assets nothing would ever serve.
//
// The router only reaches this package when web2 reports no frontend, which
// cannot happen under this build tag — see web2/assets_embed.go. The empty
// filesystem is what keeps that unreachable branch harmless rather than a nil
// dereference.
func Files() fs.FS {
	return embed.FS{}
}

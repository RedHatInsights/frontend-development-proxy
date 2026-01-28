package main

import (
	caddycmd "github.com/caddyserver/caddy/v2/cmd"

	// Import Caddy standard modules
	_ "github.com/caddyserver/caddy/v2/modules/standard"
	
	// Import custom plugins
	_ "github.com/RedHatInsights/frontend-development-proxy/feo_interceptor"
	_ "github.com/RedHatInsights/frontend-development-proxy/rh_identity_transform"
)

func main() {
	caddycmd.Main()
}

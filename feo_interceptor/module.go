package feo_interceptor

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

func init() {
	// Register the module with Caddy
	caddy.RegisterModule(FEOInterceptor{})
	
	// Register the Caddyfile directive handler
	httpcaddyfile.RegisterHandlerDirective("feo_interceptor", parseCaddyfile)
}

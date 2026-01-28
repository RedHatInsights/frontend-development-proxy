package feo_interceptor

import (
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// parseCaddyfile sets up the FEO Interceptor from Caddyfile tokens
// Syntax:
//   feo_interceptor {
//       crd_path ./deploy/frontend.yaml
//   }
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var f FEOInterceptor
	
	// Set enabled to true by default when the directive is used
	f.Enabled = true
	
	// Parse the block
	for h.Next() {
		// Handle any remaining arguments on the same line as the directive
		if h.NextArg() {
			return nil, h.ArgErr()
		}
		
		// Parse the block contents
		for h.NextBlock(0) {
			switch h.Val() {
			case "crd_path":
				if !h.NextArg() {
					return nil, h.ArgErr()
				}
				f.CRDPath = h.Val()
				if h.NextArg() {
					return nil, h.ArgErr()
				}
			case "enabled":
				if !h.NextArg() {
					return nil, h.ArgErr()
				}
				var enabled bool
				switch h.Val() {
				case "true", "on", "yes":
					enabled = true
				case "false", "off", "no":
					enabled = false
				default:
					return nil, h.Errf("enabled must be true or false, got: %s", h.Val())
				}
				f.Enabled = enabled
				if h.NextArg() {
					return nil, h.ArgErr()
				}
			default:
				return nil, h.Errf("unknown subdirective: %s", h.Val())
			}
		}
	}
	
	// Validate that crd_path is set
	if f.CRDPath == "" {
		return nil, h.Err("crd_path is required")
	}
	
	return &f, nil
}

package feo_interceptor

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dop251/goja"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

//go:embed interceptors.js
var interceptorsJS string

// FEOInterceptor implements an HTTP handler that intercepts Chrome service requests
// and merges them with local Frontend CRD configurations
type FEOInterceptor struct {
	// CRDPath is the path to the Frontend CRD YAML file
	CRDPath string `json:"crd_path,omitempty"`

	// Enabled controls whether the interceptor is active
	Enabled bool `json:"enabled,omitempty"`

	// Runtime state
	logger  *zap.Logger
	program *goja.Program
}

// CaddyModule returns the Caddy module information
func (FEOInterceptor) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.feo_interceptor",
		New: func() caddy.Module { return new(FEOInterceptor) },
	}
}

// Provision sets up the FEOInterceptor middleware
func (f *FEOInterceptor) Provision(ctx caddy.Context) error {
	f.logger = ctx.Logger(f)

	// Compile the embedded JavaScript once for better performance
	// The compiled program is thread-safe and can be reused across requests
	program, err := goja.Compile("interceptors.js", interceptorsJS, false)
	if err != nil {
		return fmt.Errorf("failed to compile interceptors.js: %w", err)
	}
	f.program = program

	// Force enable the interceptor
	f.Enabled = true

	f.logger.Info("FEO Interceptor provisioned",
		zap.String("crd_path", f.CRDPath),
		zap.Bool("enabled", f.Enabled))

	return nil
}

// registerBridgeFunctions sets up the Go functions that can be called from JavaScript
func (f *FEOInterceptor) registerBridgeFunctions(vm *goja.Runtime) error {
	// _GO_READ_FILE(path): reads a file and returns its content as string
	if err := vm.Set("_GO_READ_FILE", func(path string) (string, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			f.logger.Error("Failed to read file", zap.String("path", path), zap.Error(err))
			return "", err
		}
		return string(data), nil
	}); err != nil {
		return fmt.Errorf("failed to register _GO_READ_FILE: %w", err)
	}

	// _GO_PARSE_YAML(str): parses YAML string and returns as object
	if err := vm.Set("_GO_PARSE_YAML", func(yamlStr string) (map[string]interface{}, error) {
		var result map[string]interface{}
		if err := yaml.Unmarshal([]byte(yamlStr), &result); err != nil {
			f.logger.Error("Failed to parse YAML", zap.Error(err))
			return nil, err
		}
		return result, nil
	}); err != nil {
		return fmt.Errorf("failed to register _GO_PARSE_YAML: %w", err)
	}

	// _GO_LOG(msg): logs a message using Caddy's logger
	if err := vm.Set("_GO_LOG", func(msg string) {
		f.logger.Info("JS Log", zap.String("message", msg))
	}); err != nil {
		return fmt.Errorf("failed to register _GO_LOG: %w", err)
	}

	return nil
}

// shouldIntercept checks if the request path should be intercepted
func (f *FEOInterceptor) shouldIntercept(path string) bool {
	// Check if path contains the chrome-service static API prefix
	if !strings.Contains(path, "/api/chrome-service/v1/static/") {
		return false
	}

	// Check if path ends with any of the target JSON files
	validSuffixes := []string{
		"/bundles-generated.json",         // Navigation bundles
		"/fed-modules-generated.json",     // Module federation
		"/search-index-generated.json",    // Search index
		"/service-tiles-generated.json",   // Service tiles
		"/widget-registry-generated.json", // Widget registry
	}

	for _, suffix := range validSuffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}

	return false
}

// ServeHTTP implements caddyhttp.MiddlewareHandler
func (f *FEOInterceptor) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) (err error) {
	// Catch any panics
	defer func() {
		if rec := recover(); rec != nil {
			f.logger.Error("Panic in FEO interceptor ServeHTTP",
				zap.Any("panic", rec),
				zap.String("path", r.URL.Path))
			err = fmt.Errorf("panic in FEO interceptor: %v", rec)
		}
	}()

	// Check if this request should be intercepted
	if !f.Enabled || !f.shouldIntercept(r.URL.Path) {
		return next.ServeHTTP(w, r)
	}

	f.logger.Debug("FEO intercepting request",
		zap.String("path", r.URL.Path),
		zap.String("url", r.URL.String()))

	// Create a response recorder to capture the upstream response
	var buf bytes.Buffer
	rec := caddyhttp.NewResponseRecorder(w, &buf, func(status int, header http.Header) bool {
		return true // Buffer all responses
	})

	// Call the next handler to get the upstream response
	if err := next.ServeHTTP(rec, r); err != nil {
		f.logger.Error("Upstream handler error",
			zap.String("path", r.URL.Path),
			zap.Error(err))
		return err
	}

	// If the upstream didn't return 200, pass through the original response
	if rec.Status() != http.StatusOK {
		f.logger.Debug("Upstream returned non-200 status, passing through",
			zap.Int("status", rec.Status()),
			zap.String("path", r.URL.Path))
		return rec.WriteResponse()
	}

	// Read the upstream response body
	upstreamBody := buf.Bytes()
	f.logger.Debug("Upstream response captured",
		zap.Int("size", len(upstreamBody)),
		zap.String("path", r.URL.Path))

	// Process the request through JavaScript
	processedBody, err := f.processRequest(r.URL.String(), string(upstreamBody))
	if err != nil {
		f.logger.Error("Failed to process request in JavaScript",
			zap.String("path", r.URL.Path),
			zap.String("url", r.URL.String()),
			zap.Error(err))
		// On error, return the original response
		return rec.WriteResponse()
	}

	// Copy headers from the recorded response
	for k, v := range rec.Header() {
		w.Header()[k] = v
	}

	// Ensure content type is set
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}

	w.WriteHeader(http.StatusOK)

	// Write the processed response
	written, err := io.WriteString(w, processedBody)
	if err != nil {
		f.logger.Error("Failed to write processed response",
			zap.String("path", r.URL.Path),
			zap.Error(err))
		return err
	}

	f.logger.Info("Request intercepted and processed successfully",
		zap.String("path", r.URL.Path),
		zap.Int("original_size", len(upstreamBody)),
		zap.Int("processed_size", len(processedBody)),
		zap.Int("written", written))

	return nil
}

// processRequest calls the JavaScript processRequest function
// Creates a new VM instance per request to avoid concurrency issues
func (f *FEOInterceptor) processRequest(url, upstreamBody string) (resultStr string, resultErr error) {
	// Recover from any panics in JavaScript execution
	defer func() {
		if r := recover(); r != nil {
			f.logger.Error("Panic in JavaScript execution",
				zap.Any("panic", r),
				zap.String("url", url))
			resultErr = fmt.Errorf("panic in JavaScript execution: %v", r)
			resultStr = upstreamBody // Return original on panic
		}
	}()

	// Create a new VM for this request (goja.Runtime is NOT thread-safe)
	vm := goja.New()

	// Register bridge functions for this VM
	if err := f.registerBridgeFunctions(vm); err != nil {
		f.logger.Error("Failed to register bridge functions",
			zap.Error(err),
			zap.String("url", url))
		return "", fmt.Errorf("failed to register bridge functions: %w", err)
	}

	// Run the compiled program to load function definitions
	if _, err := vm.RunProgram(f.program); err != nil {
		f.logger.Error("Failed to execute interceptors.js",
			zap.Error(err),
			zap.String("url", url))
		return "", fmt.Errorf("failed to execute interceptors.js: %w", err)
	}

	// Get the processRequest function from the VM
	processRequestFn, ok := goja.AssertFunction(vm.Get("processRequest"))
	if !ok {
		f.logger.Error("processRequest function not found in JavaScript runtime",
			zap.String("url", url))
		return "", fmt.Errorf("processRequest function not found in JavaScript runtime")
	}

	f.logger.Debug("Calling JavaScript processRequest",
		zap.String("url", url),
		zap.String("crd_path", f.CRDPath),
		zap.Int("upstream_body_size", len(upstreamBody)))

	// Call the JavaScript function: processRequest(url, upstreamBody, crdPath)
	result, err := processRequestFn(goja.Undefined(),
		vm.ToValue(url),
		vm.ToValue(upstreamBody),
		vm.ToValue(f.CRDPath))
	if err != nil {
		f.logger.Error("JavaScript processRequest failed",
			zap.Error(err),
			zap.String("url", url))
		return "", fmt.Errorf("JavaScript processRequest failed: %w", err)
	}

	// Convert the result to a string
	resultStr = result.String()
	f.logger.Debug("JavaScript processRequest completed",
		zap.String("url", url),
		zap.Int("result_size", len(resultStr)))

	return resultStr, nil
}

// Interface guards
var (
	_ caddy.Provisioner           = (*FEOInterceptor)(nil)
	_ caddyhttp.MiddlewareHandler = (*FEOInterceptor)(nil)
)

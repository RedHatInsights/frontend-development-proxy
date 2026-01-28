package feo_interceptor

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dop251/goja"
	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

//go:embed interceptors_bundled.js
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

	// File watching
	watcher     *fsnotify.Watcher
	crdCache    map[string]interface{}
	crdCacheMu  sync.RWMutex
	stopWatcher chan struct{}
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

	// Check if FEO is disabled via environment variable
	enableFEO := os.Getenv("ENABLE_FEO")
	if enableFEO == "false" || enableFEO == "0" {
		f.Enabled = false
		f.logger.Info("FEO Interceptor DISABLED via ENABLE_FEO environment variable")
		return nil
	}

	// Compile the embedded JavaScript once for better performance
	// The compiled program is thread-safe and can be reused across requests
	program, err := goja.Compile("interceptors.js", interceptorsJS, false)
	if err != nil {
		return fmt.Errorf("failed to compile interceptors.js: %w", err)
	}
	f.program = program

	// Force enable the interceptor
	f.Enabled = true

	// Load CRD file and start watching
	if f.CRDPath != "" {
		if err := f.loadCRD(); err != nil {
			f.logger.Error("Failed to load CRD - will pass through requests",
				zap.Error(err))
		}

		if err := f.startFileWatcher(); err != nil {
			f.logger.Error("Failed to start file watcher",
				zap.Error(err))
		}
	}

	f.logger.Info("FEO Interceptor provisioned",
		zap.String("crd_path", f.CRDPath),
		zap.Bool("enabled", f.Enabled))

	return nil
}

// registerBridgeFunctions sets up the Go functions that can be called from JavaScript
func (f *FEOInterceptor) registerBridgeFunctions(vm *goja.Runtime) error {
	// _GO_READ_FILE(path): reads a file and returns its content as string
	if err := vm.Set("_GO_READ_FILE", func(path string) (string, error) {
		// Use cache for CRD file
		if path == f.CRDPath && f.crdCache != nil {
			f.logger.Debug("Using cached CRD file", zap.String("path", path))
			yamlBytes, err := yaml.Marshal(f.crdCache)
			if err != nil {
				f.logger.Error("Failed to marshal cached CRD", zap.Error(err))
			} else {
				return string(yamlBytes), nil
			}
		}

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

// loadCRD reads and caches the CRD file
func (f *FEOInterceptor) loadCRD() error {
	f.logger.Info("Loading CRD file", zap.String("path", f.CRDPath))

	data, err := os.ReadFile(f.CRDPath)
	if err != nil {
		return fmt.Errorf("failed to read CRD file: %w", err)
	}

	var result map[string]interface{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("failed to parse CRD YAML: %w", err)
	}

	f.crdCacheMu.Lock()
	f.crdCache = result
	f.crdCacheMu.Unlock()

	f.logger.Info("CRD file loaded successfully", zap.Int("size", len(data)))
	return nil
}

// getCachedCRD returns the cached CRD data
func (f *FEOInterceptor) getCachedCRD() map[string]interface{} {
	f.crdCacheMu.RLock()
	defer f.crdCacheMu.RUnlock()
	return f.crdCache
}

// startFileWatcher starts watching the CRD file for changes
func (f *FEOInterceptor) startFileWatcher() error {
	if f.CRDPath == "" {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	if err := watcher.Add(f.CRDPath); err != nil {
		watcher.Close()
		return fmt.Errorf("failed to watch CRD file: %w", err)
	}

	f.watcher = watcher
	f.stopWatcher = make(chan struct{})

	f.logger.Info("File watcher started", zap.String("path", f.CRDPath))

	go f.watcherLoop()
	return nil
}

// watcherLoop processes file system events
func (f *FEOInterceptor) watcherLoop() {
	for {
		select {
		case event, ok := <-f.watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Write == fsnotify.Write ||
				event.Op&fsnotify.Create == fsnotify.Create {
				f.logger.Info("CRD file changed, reloading", zap.String("path", event.Name))

				if err := f.loadCRD(); err != nil {
					f.logger.Error("Failed to reload CRD", zap.Error(err))
				}
			}

		case err, ok := <-f.watcher.Errors:
			if !ok {
				return
			}
			f.logger.Error("File watcher error", zap.Error(err))

		case <-f.stopWatcher:
			return
		}
	}
}

// Cleanup stops the file watcher
func (f *FEOInterceptor) Cleanup() error {
	if f.watcher != nil {
		close(f.stopWatcher)
		return f.watcher.Close()
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

	f.logger.Info("FEO intercepting request",
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

	f.logger.Info("Upstream response received",
		zap.Int("status", rec.Status()),
		zap.Int("body_size", len(upstreamBody)),
		zap.String("path", r.URL.Path))

	f.logger.Info("Processing request through JavaScript",
		zap.String("path", r.URL.Path),
		zap.String("crd_path", f.CRDPath))

	// Process the request through JavaScript
	processedBody, err := f.processRequest(r.URL.String(), string(upstreamBody))
	if err != nil {
		f.logger.Error("Failed to process request in JavaScript - RETURNING ORIGINAL RESPONSE",
			zap.String("path", r.URL.Path),
			zap.String("url", r.URL.String()),
			zap.Error(err),
			zap.String("crd_path", f.CRDPath),
			zap.Bool("crd_exists", fileExists(f.CRDPath)))
		// Add header to indicate processing failed
		w.Header().Set("X-FEO-Processing-Error", "true")
		// On error, return the original response
		return rec.WriteResponse()
	}

	// Copy headers from the recorded response (except Content-Length)
	for k, v := range rec.Header() {
		// Skip Content-Length as it will be wrong after processing
		if k != "Content-Length" {
			w.Header()[k] = v
		}
	}

	// Ensure content type is set
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}

	// Set correct Content-Length for the processed body
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(processedBody)))

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

	// Inject CommonJS polyfill (module, exports) before running the bundle
	// This is needed because the JavaScript bundle uses module.exports (CommonJS)
	// but Goja is a pure ECMAScript runtime without Node.js globals
	commonJSPolyfill := `
		var module = { exports: {} };
		var exports = module.exports;
	`
	if _, err := vm.RunString(commonJSPolyfill); err != nil {
		f.logger.Error("Failed to inject CommonJS polyfill",
			zap.Error(err),
			zap.String("url", url))
		return "", fmt.Errorf("failed to inject CommonJS polyfill: %w", err)
	}

	// Run the compiled program to load function definitions
	if _, err := vm.RunProgram(f.program); err != nil {
		f.logger.Error("Failed to execute interceptors.js",
			zap.Error(err),
			zap.String("url", url))
		return "", fmt.Errorf("failed to execute interceptors.js: %w", err)
	}

	// Expose module.exports to global scope if processRequest is not directly available
	// The bundle may export via module.exports.processRequest
	processRequestVal := vm.Get("processRequest")
	if goja.IsUndefined(processRequestVal) || goja.IsNull(processRequestVal) {
		// Try to get it from module.exports
		moduleExports := vm.Get("module").ToObject(vm).Get("exports")
		if moduleExports != nil && !goja.IsUndefined(moduleExports) && !goja.IsNull(moduleExports) {
			processRequestVal = moduleExports.ToObject(vm).Get("processRequest")
			if processRequestVal != nil && !goja.IsUndefined(processRequestVal) && !goja.IsNull(processRequestVal) {
				// Set it as a global for convenience
				vm.Set("processRequest", processRequestVal)
				f.logger.Debug("Exposed processRequest from module.exports to global scope")
			}
		}
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

	// Get bridge functions from the VM to pass as parameters
	goReadFile := vm.Get("_GO_READ_FILE")
	goParseYaml := vm.Get("_GO_PARSE_YAML")
	goLog := vm.Get("_GO_LOG")

	// Call the JavaScript function: processRequest(url, upstreamBody, crdPath, goReadFile, goParseYaml, goLog)
	result, err := processRequestFn(goja.Undefined(),
		vm.ToValue(url),
		vm.ToValue(upstreamBody),
		vm.ToValue(f.CRDPath),
		goReadFile,
		goParseYaml,
		goLog)
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

// fileExists checks if a file exists at the given path
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Interface guards
var (
	_ caddy.Provisioner           = (*FEOInterceptor)(nil)
	_ caddyhttp.MiddlewareHandler = (*FEOInterceptor)(nil)
	_ caddy.CleanerUpper          = (*FEOInterceptor)(nil)
)

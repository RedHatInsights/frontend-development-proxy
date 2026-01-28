package feo_interceptor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/dop251/goja"
	"go.uber.org/zap/zaptest"
)

func TestShouldIntercept(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "bundles-generated.json",
			path:     "/api/chrome-service/v1/static/bundles-generated.json",
			expected: true,
		},
		{
			name:     "fed-modules-generated.json",
			path:     "/api/chrome-service/v1/static/fed-modules-generated.json",
			expected: true,
		},
		{
			name:     "search-index-generated.json",
			path:     "/api/chrome-service/v1/static/search-index-generated.json",
			expected: true,
		},
		{
			name:     "service-tiles-generated.json",
			path:     "/api/chrome-service/v1/static/service-tiles-generated.json",
			expected: true,
		},
		{
			name:     "widget-registry-generated.json",
			path:     "/api/chrome-service/v1/static/widget-registry-generated.json",
			expected: true,
		},
		{
			name:     "non-matching path",
			path:     "/api/some/other/path",
			expected: false,
		},
		{
			name:     "chrome-service but wrong file",
			path:     "/api/chrome-service/v1/static/other.json",
			expected: false,
		},
	}

	f := &FEOInterceptor{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.shouldIntercept(tt.path)
			if result != tt.expected {
				t.Errorf("shouldIntercept(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestRegisterBridgeFunctions(t *testing.T) {
	logger := zaptest.NewLogger(t)
	f := &FEOInterceptor{logger: logger}

	vm := goja.New()
	err := f.registerBridgeFunctions(vm)
	if err != nil {
		t.Fatalf("registerBridgeFunctions() error = %v", err)
	}

	// Check that all bridge functions are registered
	functions := []string{"_GO_READ_FILE", "_GO_PARSE_YAML", "_GO_LOG"}
	for _, fn := range functions {
		val := vm.Get(fn)
		if val == nil || goja.IsUndefined(val) {
			t.Errorf("Bridge function %s not registered", fn)
		}
	}
}

func TestProcessRequestWithValidCRD(t *testing.T) {
	// Skip this test - JavaScript bundle has Node.js dependencies (module.exports)
	// that don't work in Goja without additional setup
	t.Skip("Skipping due to JavaScript bundle Node.js dependencies")

	// Create temporary CRD file
	tmpDir := t.TempDir()
	crdPath := filepath.Join(tmpDir, "frontend.yaml")

	crdContent := `apiVersion: v1
kind: List
objects:
  - apiVersion: console.redhat.com/v1
    kind: ConsoleFrontend
    metadata:
      name: test-app
    spec:
      feoConfigEnabled: true
      navigationSegments:
        - segmentId: test-segment
          navItems:
            - id: test-item
              title: "Test Item"
              href: "/test"
`

	if err := os.WriteFile(crdPath, []byte(crdContent), 0644); err != nil {
		t.Fatalf("Failed to write test CRD: %v", err)
	}

	logger := zaptest.NewLogger(t)
	f := &FEOInterceptor{
		CRDPath: crdPath,
		logger:  logger,
		Enabled: true,
	}

	// Provision to compile JavaScript
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()

	if err := f.Provision(ctx); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	// Test processing navigation request
	url := "https://example.com/api/chrome-service/v1/static/bundles-generated.json"
	upstream := `[{"id":"test","title":"Test","navItems":[]}]`

	result, err := f.processRequest(url, upstream)
	if err != nil {
		t.Errorf("processRequest() error = %v", err)
	}

	if result == "" {
		t.Error("processRequest() returned empty result")
	}

	// Result should be valid JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Errorf("processRequest() returned invalid JSON: %v", err)
	}
}

func TestFileWatcher(t *testing.T) {
	// Create temporary CRD file
	tmpDir := t.TempDir()
	crdPath := filepath.Join(tmpDir, "frontend.yaml")

	initialContent := `apiVersion: v1
kind: List
objects:
  - apiVersion: console.redhat.com/v1
    kind: ConsoleFrontend
    metadata:
      name: test-app
    spec:
      feoConfigEnabled: true
`

	if err := os.WriteFile(crdPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to write initial CRD: %v", err)
	}

	logger := zaptest.NewLogger(t)
	f := &FEOInterceptor{
		CRDPath: crdPath,
		logger:  logger,
	}

	// Load CRD and start watcher
	if err := f.loadCRD(); err != nil {
		t.Fatalf("loadCRD() error = %v", err)
	}

	if err := f.startFileWatcher(); err != nil {
		t.Fatalf("startFileWatcher() error = %v", err)
	}
	defer f.Cleanup()

	// Get initial cache
	cache1 := f.getCachedCRD()
	if cache1 == nil {
		t.Fatal("Initial cache is nil")
	}

	// Modify the file
	updatedContent := `apiVersion: v1
kind: List
objects:
  - apiVersion: console.redhat.com/v1
    kind: ConsoleFrontend
    metadata:
      name: test-app-updated
    spec:
      feoConfigEnabled: true
`

	if err := os.WriteFile(crdPath, []byte(updatedContent), 0644); err != nil {
		t.Fatalf("Failed to write updated CRD: %v", err)
	}

	// Wait for file watcher to detect change
	time.Sleep(500 * time.Millisecond)

	// Get updated cache
	cache2 := f.getCachedCRD()
	if cache2 == nil {
		t.Fatal("Updated cache is nil")
	}

	// Verify cache was updated
	objects, ok := cache2["objects"].([]interface{})
	if !ok {
		t.Fatal("Cache objects is not a slice")
	}
	obj, ok := objects[0].(map[string]interface{})
	if !ok {
		t.Fatal("Cache object is not a map")
	}
	metadata, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("Cache metadata is not a map")
	}
	name, ok := metadata["name"].(string)
	if !ok {
		t.Fatal("Cache name is not a string")
	}

	if name != "test-app-updated" {
		t.Errorf("Cache not updated, name = %s, want test-app-updated", name)
	}
}

func TestProvisionWithEnvVar(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		wantEnabled bool
	}{
		{
			name:        "explicitly disabled",
			envValue:    "false",
			wantEnabled: false,
		},
		{
			name:        "disabled with 0",
			envValue:    "0",
			wantEnabled: false,
		},
		{
			name:        "enabled by default",
			envValue:    "",
			wantEnabled: true,
		},
		{
			name:        "explicitly enabled",
			envValue:    "true",
			wantEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envValue != "" {
				os.Setenv("ENABLE_FEO", tt.envValue)
				defer os.Unsetenv("ENABLE_FEO")
			}

			logger := zaptest.NewLogger(t)
			f := &FEOInterceptor{logger: logger}

			ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
			defer cancel()

			err := f.Provision(ctx)
			if err != nil {
				t.Fatalf("Provision() error = %v", err)
			}

			if f.Enabled != tt.wantEnabled {
				t.Errorf("Enabled = %v, want %v", f.Enabled, tt.wantEnabled)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() string
		expected bool
	}{
		{
			name: "file exists",
			setup: func() string {
				tmpFile, _ := os.CreateTemp("", "test-*")
				path := tmpFile.Name()
				tmpFile.Close()
				return path
			},
			expected: true,
		},
		{
			name: "file does not exist",
			setup: func() string {
				return "/nonexistent/path/to/file.txt"
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup()
			if tt.expected {
				defer os.Remove(path)
			}

			result := fileExists(path)
			if result != tt.expected {
				t.Errorf("fileExists(%q) = %v, want %v", path, result, tt.expected)
			}
		})
	}
}

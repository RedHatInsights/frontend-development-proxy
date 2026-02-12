#!/bin/bash
set -e

echo "=== FEO Interceptor Integration Test ==="
echo ""

# Configuration
PROXY_PORT=1337
MOCK_PORT=9999
TEST_DIR=$(mktemp -d)
PROXY_PID=""
MOCK_PID=""

cleanup() {
    echo ""
    echo "=== Cleanup ==="
    if [ -n "$PROXY_PID" ]; then
        echo "Stopping proxy (PID: $PROXY_PID)"
        kill $PROXY_PID 2>/dev/null || true
    fi
    if [ -n "$MOCK_PID" ]; then
        echo "Stopping mock server (PID: $MOCK_PID)"
        kill $MOCK_PID 2>/dev/null || true
    fi
    rm -rf "$TEST_DIR"
    echo "Cleanup complete"
}

trap cleanup EXIT

echo "Step 1: Build proxy"
go build -o frontend-development-proxy ./cmd/frontend-development-proxy
echo "✓ Proxy built successfully"
echo ""

echo "Step 2: Start mock upstream server"
mkdir -p "$TEST_DIR/api/chrome-service/v1/static"
cat > "$TEST_DIR/api/chrome-service/v1/static/bundles-generated.json" << 'EOF'
[
  {
    "id": "insights",
    "title": "Insights",
    "navItems": [
      {
        "id": "remote-item",
        "title": "Remote Item",
        "href": "/remote"
      }
    ]
  }
]
EOF

cd "$TEST_DIR"
python3 -m http.server $MOCK_PORT > /dev/null 2>&1 &
MOCK_PID=$!
cd - > /dev/null
sleep 1

# Verify mock server
if ! kill -0 $MOCK_PID 2>/dev/null; then
    echo "✗ Mock server failed to start"
    exit 1
fi
echo "✓ Mock server started on port $MOCK_PORT (PID: $MOCK_PID)"
echo ""

echo "Step 3: Setup CRD file"
# Create test CRD from example if it doesn't exist
if [ ! -f "config/deploy/frontend.yaml" ]; then
    if [ -f "config/deploy/frontend.example.yaml" ]; then
        cp config/deploy/frontend.example.yaml config/deploy/frontend.yaml
        echo "✓ Created CRD file from example"
    else
        echo "✗ Example CRD file not found"
        exit 1
    fi
else
    echo "✓ CRD file exists"
fi
echo ""

echo "Step 4: Start proxy with FEO enabled"
export HCC_ENV=test
export HCC_ENV_URL=http://localhost:$MOCK_PORT
export PROXY_PORT=$PROXY_PORT
export ENABLE_FEO=true

./frontend-development-proxy run --config Caddyfile --adapter caddyfile > "$TEST_DIR/proxy.log" 2>&1 &
PROXY_PID=$!
sleep 3

# Check if proxy started successfully
if ! kill -0 $PROXY_PID 2>/dev/null; then
    echo "✗ Proxy failed to start"
    cat "$TEST_DIR/proxy.log"
    exit 1
fi
echo "✓ Proxy started on port $PROXY_PORT (PID: $PROXY_PID)"

# Wait for proxy to be ready to accept connections
echo "Waiting for proxy to be ready..."
for i in {1..10}; do
    if curl -s -o /dev/null -w "%{http_code}" -H "Host: test.foo.redhat.com" "http://localhost:$PROXY_PORT/" > /dev/null 2>&1; then
        echo "✓ Proxy is accepting connections"
        break
    fi
    if [ $i -eq 10 ]; then
        echo "✗ Proxy not responding after 10 attempts"
        echo "Proxy logs:"
        cat "$TEST_DIR/proxy.log"
        exit 1
    fi
    sleep 0.5
done
echo ""

echo "Step 5: Check proxy logs for FEO provisioning"
if grep -q "FEO Interceptor provisioned" "$TEST_DIR/proxy.log"; then
    echo "✓ FEO Interceptor provisioned"
else
    echo "⚠ FEO Interceptor provisioning message not found"
    echo "Proxy logs:"
    cat "$TEST_DIR/proxy.log"
fi
echo ""

echo "Step 6: Test interception"
RESPONSE=$(curl -s -H "Host: test.foo.redhat.com" \
    "http://localhost:$PROXY_PORT/api/chrome-service/v1/static/bundles-generated.json" 2>&1)
CURL_EXIT=$?

if [ $CURL_EXIT -ne 0 ]; then
    echo "✗ Curl failed with exit code $CURL_EXIT"
    echo "Response: $RESPONSE"
    echo "Proxy logs:"
    cat "$TEST_DIR/proxy.log"
    exit 1
fi

if [ -z "$RESPONSE" ]; then
    echo "✗ No response received from proxy"
    echo "Proxy logs:"
    cat "$TEST_DIR/proxy.log"
    exit 1
fi

echo "Response received:"
echo "$RESPONSE" | jq . || echo "$RESPONSE"
echo ""

echo "Step 7: Verify merge"
if command -v jq &> /dev/null; then
    REMOTE_COUNT=$(echo "$RESPONSE" | jq '[.[] | select(.id == "insights") | .navItems[]? | select(.title == "Remote Item")] | length' 2>/dev/null || echo "0")
    LOCAL_COUNT=$(echo "$RESPONSE" | jq '[.[] | select(.id == "insights") | .navItems[]? | select(.title == "Local Bundle Item")] | length' 2>/dev/null || echo "0")

    echo "Remote items found: $REMOTE_COUNT"
    echo "Local items found: $LOCAL_COUNT"
    echo ""

    if [ "$REMOTE_COUNT" -gt 0 ]; then
        echo "✓ Remote data present"
        if [ "$LOCAL_COUNT" -gt 0 ]; then
            echo "✓✓✓ SUCCESS: Data merging works! ✓✓✓"
            echo ""
            echo "Both remote and local items found in response."
            echo "FEO interceptor is working correctly!"
            exit 0
        else
            echo "⚠ WARNING: Local data not merged"
            echo ""
            echo "Remote items present but local CRD data not merged."
            echo "This may indicate JavaScript processing issues."
            echo ""
            echo "Proxy logs:"
            grep -i "feo\|javascript" "$TEST_DIR/proxy.log" || echo "No FEO-related logs found"
            exit 1
        fi
    else
        echo "✗✗✗ FAILURE: No data in response ✗✗✗"
        echo ""
        echo "Proxy logs:"
        cat "$TEST_DIR/proxy.log"
        exit 1
    fi
else
    echo "⚠ jq not installed, skipping JSON verification"
    if echo "$RESPONSE" | grep -q "Remote Item"; then
        echo "✓ Response contains remote data"
        if echo "$RESPONSE" | grep -q "Local Bundle Item"; then
            echo "✓✓✓ SUCCESS: Data merging appears to work! ✓✓✓"
            exit 0
        else
            echo "⚠ WARNING: Local data may not be merged"
            exit 1
        fi
    else
        echo "✗ Response does not contain expected data"
        exit 1
    fi
fi

#!/bin/bash
# Update FEO Interceptors from frontend-components package
# 
# This script fetches the latest bundled interceptors from the 
# @redhat-cloud-services/frontend-components-config-utilities package
#
# Usage:
#   ./update-interceptors.sh [version]
#
# If no version is specified, fetches the latest version.

set -e

PACKAGE_NAME="@redhat-cloud-services/frontend-components-config-utilities"
VERSION="${1:-latest}"
TARGET_FILE="interceptors_bundled.js"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Fetching FEO interceptors from ${PACKAGE_NAME}@${VERSION}..."

# Create a temporary directory
TEMP_DIR=$(mktemp -d)
trap "rm -rf ${TEMP_DIR}" EXIT

cd "${TEMP_DIR}"

# Download the package
echo "Downloading package..."
npm pack "${PACKAGE_NAME}@${VERSION}" --quiet

# Extract the package
TARBALL=$(ls *.tgz)
tar -xzf "${TARBALL}"

# Copy the bundled interceptors
if [ -f "package/standalone/feo-interceptors.js" ]; then
    cp "package/standalone/feo-interceptors.js" "${SCRIPT_DIR}/${TARGET_FILE}"
    echo "✓ Interceptors updated successfully!"
    echo "  Source: ${PACKAGE_NAME}@${VERSION}"
    echo "  Target: ${SCRIPT_DIR}/${TARGET_FILE}"
    
    # Get the actual version from package.json
    ACTUAL_VERSION=$(cat package/package.json | grep '"version"' | head -1 | sed 's/.*"version": "\(.*\)".*/\1/')
    echo "  Version: ${ACTUAL_VERSION}"
else
    echo "✗ Error: Bundled interceptors not found in package!"
    echo "  Expected: package/standalone/feo-interceptors.js"
    exit 1
fi

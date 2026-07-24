#!/bin/sh
set -e

# Fetch latest version tag from GitHub API
VERSION=$(curl -s "https://api.github.com/repos/divamtech/MiniShare/releases/latest" | grep '"tag_name":' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')

if [ -z "$VERSION" ] || [ "$VERSION" = "null" ]; then
    echo "Error: Could not retrieve the latest release version from GitHub."
    exit 1
fi

echo "Latest Version: $VERSION"

# stop and delete existing PM2 process before replacing the binary,
# otherwise unzip fails with "Text file busy" on the running server
pm2 stop minishare-server || true
pm2 delete minishare-server || true

# download and extract
curl -L -O "https://github.com/divamtech/MiniShare/releases/download/$VERSION/server-linux-$VERSION.zip"
rm -rf minishare-server
unzip -o "server-linux-$VERSION.zip" -d minishare-server

# clean up zip
rm -rf "server-linux-$VERSION.zip"

cd minishare-server
chmod +x server

# start server under PM2
pm2 start ./server --name "minishare-server" -- --port 8080

# test locally
# curl -I http://localhost:8080
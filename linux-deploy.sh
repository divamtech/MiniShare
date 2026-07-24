#!/bin/sh

sudo apt-get update
sudo apt-get install -y unzip curl

# Fetch latest version tag from GitHub API
VERSION=$(curl -s "https://api.github.com/repos/divamtech/MiniShare/releases/latest" | grep '"tag_name":' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')

if [ -z "$VERSION" ] || [ "$VERSION" = "null" ]; then
    echo "Error: Could not retrieve the latest release version from GitHub."
    exit 1
fi

echo "Latest Version: $VERSION"

curl -L -O "https://github.com/divamtech/MiniShare/releases/download/$VERSION/server-linux-$VERSION.zip"
unzip "server-linux-$VERSION.zip" -d minishare-server

# clean up zip
rm -rf "server-linux-$VERSION.zip"

cd minishare-server
chmod +x server

# test the server is running locally.
# ./server --port 8080

pm2 start ./server --name "minishare-server" -- --port 8080
#---or
# nohup ./server --port 8080 > server.log 2>&1 &


# test the deployment
# curl -I http://localhost:8080
# curl http://localhost:8080
# curl -I https://minishare.divamtech.com
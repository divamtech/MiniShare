#!/bin/sh

sudo apt-get update
sudo apt-get install -y unzip curl

curl -L -O https://github.com/divamtech/MiniShare/releases/download/v1.2.0/server-linux-v1.2.0.zip
unzip server-linux-v1.2.0.zip -d minishare-server
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
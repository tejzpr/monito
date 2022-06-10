[![Open Source](https://img.shields.io/badge/Open%20Source-%20-green?logo=open-source-initiative&logoColor=white&color=blue&labelColor=blue)](https://en.wikipedia.org/wiki/Open_source)
[![Golang](https://img.shields.io/badge/-Go%20Lang-blue?logo=go&logoColor=white)](https://golang.org)
[![Go Report Card](https://goreportcard.com/badge/github.com/tejzpr/monito)](https://goreportcard.com/report/github.com/tejzpr/monito)
[![CodeQL](https://github.com/tejzpr/monito/actions/workflows/codeql-analysis.yml/badge.svg?branch=main)](https://github.com/tejzpr/monito/actions/workflows/codeql-analysis.yml)

# Monito
![Monito](https://github.com/tejzpr/monito/blob/main/public/static/favicon/favicon-32x32.png?raw=true)

Monito provides a configuration based light weight remote server status page with liveness monitoring and notifications. 

# Setup
Download and install a [release executable](https://github.com/tejzpr/monito/releases) and update the config/config.json file.

# Setup using Docker
Update config.json file and save it in a secure location. The following command assumes that the config file is in **/data/config/config.json** After running the command Monito will be available on port 8080
```docker
docker run -v /data/config/config.json:/app/config/config.json -p 8080:8430 ghcr.io/tejzpr/monito:latest
```

# Features
* Support for notifications via Email, Webex or almost any notification platfrom.
* Support for dynamic dashboards via Prometheus
* On / Off status for a wide variety of endpoints
* Public real-time status page

# Screenshot
![Screenshot](https://github.com/tejzpr/monito/blob/main/screenshots/sshot-1.png?raw=true)

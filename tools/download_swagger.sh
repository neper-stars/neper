#!/bin/sh
PLAT=$(uname | tr '[:upper:]' '[:lower:]') &&\
JQ_FILTER=$(printf '.assets[] | select(.name | contains("%s_amd64")) | .browser_download_url' "$PLAT") &&\
SWAGGER_RELEASE_PAGE=$(printf "https://api.github.com/repos/go-swagger/go-swagger/releases/tags/%s" "$1") &&\
DOWNLOAD_URL=$(curl -s "$SWAGGER_RELEASE_PAGE"| jq -r "$JQ_FILTER") && \
curl -o "$2" -L'#' "$DOWNLOAD_URL" &&\
chmod +x "$2"
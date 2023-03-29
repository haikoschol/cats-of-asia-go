#!/usr/bin/env bash

set -e

if [[ -z ${COABOT_SERVER} ]];
then
    echo "COABOT_SERVER env var missing"
	exit 1
fi

GOOS=linux GOARCH=amd64 go build github.com/haikoschol/cats-of-asia/cmd/coabot
scp coabot ${COABOT_SERVER}:/usr/local/bin/coabot
ssh -t ${COABOT_SERVER} "sudo systemctl restart coabot"


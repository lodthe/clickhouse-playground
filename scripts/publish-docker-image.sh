#!/bin/bash

set -e

IMAGE=$1

if [ -z "$1" ]; then
    echo 'Missed image name'
fi

docker build -t "$IMAGE" .
docker push "$IMAGE"
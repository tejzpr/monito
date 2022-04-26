#!/bin/bash
rm -rf dist
mkdir dist

export version=$(git log -n1 --format=format:"%H")
echo $version > dist/version.txt
env GOOS=linux GOARCH=amd64 go build -a -tags netgo -buildmode=pie -trimpath
mv monito ./dist/monito_linux_x86_64
env GOOS=linux GOARCH=arm64 go build -a -tags netgo -buildmode=pie -trimpath
mv monito ./dist/monito_linux_arm64
env GOOS=windows GOARCH=amd64 go build -a -tags netgo -buildmode=pie -trimpath
mv monito.exe ./dist/monito.exe
env GOOS=darwin GOARCH=amd64 go build -a -tags netgo -buildmode=pie -trimpath
mv monito ./dist/monito_darwin_x86_64
env GOOS=darwin GOARCH=arm64 go build -a -tags netgo -buildmode=pie -trimpath
mv monito ./dist/monito_darwin_arm64

cd dist

for i in monito*; do tar -czf $i.tar.gz $i; done
#!/bin/sh -x

echo -n "Compiling..."
mkdir -p build
FILENAME=sql-proxy
GOOS=linux go build -o build/main $FILENAME.go || { echo "go build failed"; exit 2; }

echo -n "Zipping..."
cp $FILENAME.go build/$FILENAME.go
cd build
zip $FILENAME.zip main $FILENAME.go

cp $FILENAME.zip ../sql-proxy.zip
cp $FILENAME.zip ../sql.zip
cp $FILENAME.zip sql.zip

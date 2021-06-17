SHELL := /bin/bash

build:
	pushd v1/circle; go build -o ../../p2p-circle ;popd

run: build
	./p2p-circle

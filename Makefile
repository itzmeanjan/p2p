SHELL := /bin/bash

build_circle:
	pushd v1/circle; go build -o ../../p2p-circle ;popd

run_circle: build_circle
	./p2p-circle

build_mesh:
	pushd v1/mesh; go build -o ../../p2p-mesh ;popd

run_mesh: build_mesh
	./p2p-mesh

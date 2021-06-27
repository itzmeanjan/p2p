SHELL := /bin/bash

build_circle:
	pushd v1/circle; go build -o ../../p2p-circle; popd

run_circle: build_circle
	./p2p-circle

build_mesh:
	pushd v1/mesh; go build -o ../../p2p-mesh; popd

run_mesh: build_mesh
	./p2p-mesh

build_starfish:
	pushd v1/starfish; go build -o ../../p2p-starfish; popd

run_starfish: build_starfish
	./p2p-starfish

build_v2:
	pushd v2; go build -o ../v2_node; popd

run_v2: build_v2
	./v2_node

gen_graph:
	find . -maxdepth 1 -name '*.dot' | xargs dot -Tpng -s144 -O
	pushd v2/scripts; python3 traffic.py; popd
	mv v2/scripts/traffic.png cost_v2.png

remove_graph:
	find . -maxdepth 1 -name '*.png' | xargs rm -v

remove_log:
	find . -maxdepth 1 -name '*.dot' | xargs rm -v
	find . -maxdepth 1 -name '*.csv' | xargs rm -v

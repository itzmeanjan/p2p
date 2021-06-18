#!/usr/bin/python3

from os import name, walk
from os.path import join, abspath
from re import compile as reg_compile
from typing import List, Tuple
from json import loads
from pyvis.network import Network


def get_file_paths(root: str) -> List[Tuple[int, str]]:
    reg = reg_compile(r'^((\d{1,})\.txt)$')
    paths = []
    for (d, _, files) in walk(root):
        for file in files:
            m = reg.match(file)
            if m:
                paths.append((int(m.group(2)), abspath(join(d, file))))
    return paths


def read_and_plot(self_id: int, file: str):
    nt = Network(height='500px', width='100%',
                 bgcolor='#232323', font_color='white',
                 heading=f'P2P Network as seen by Peer {self_id}')
    nt.barnes_hut()
    nt.add_node(self_id, label=f'Peer {self_id}')

    def process_hops(hops: List[int]):
        peer = hops[-1]
        nt.add_node(peer, label=f'Peer {peer}')
        nt.add_edge(self_id, peer)
        if len(hops) == 1:
            return

        for i, _ in enumerate(hops):
            if i == len(hops) - 1:
                break

            nt.add_node(hops[i], label=f'Peer {hops[i]}')
            nt.add_node(hops[i+1], label=f'Peer {hops[i+1]}')
            nt.add_edge(hops[i], hops[i+1])

    with open(file, 'r') as fd:
        while True:
            ln = fd.readline()
            if not ln:
                break
            msg = loads(ln.strip())
            process_hops(msg['hops'])

    nt.save_graph(f'p2p_graph_{self_id}.html')


def main():
    print('Processing p2p chit-chat log ...')
    for (peer, path) in get_file_paths('../../'):
        read_and_plot(peer, path)
    print('Generated p2p graphs !')


if __name__ == '__main__':
    main()

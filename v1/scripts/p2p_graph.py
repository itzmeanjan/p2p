#!/usr/bin/python3

from os import walk
from os.path import join, abspath
from re import compile as reg_compile
from typing import Dict, List, Tuple
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


def read_and_plot(self_id: int, traffic_log: str, message_log: str):
    nt = Network(height='720px', width='72%',
                 bgcolor='#232323', font_color='white',
                 heading=f'P2P Network as perceived by `Peer {self_id}`')
    nt.add_node(self_id, label=f'Peer {self_id}')

    def read_traffic() -> Dict[int, Tuple[int, int]]:
        traffic = {}
        with open(traffic_log, 'r') as fd:
            while True:
                ln = fd.readline()
                if not ln:
                    break
                vals = [int(i.strip()) for i in ln.split(';')]
                traffic[vals[0]] = (vals[1], vals[2])
        return traffic

    traffic = read_traffic()

    def process_hops(hops: List[int]):
        peer = hops[-1]
        weight = traffic[peer]
        nt.add_node(peer, label=f'Peer {peer}')
        nt.add_edge(self_id, peer, value=sum(weight),
                    title=f'In: {weight[0]} bytes | Out: {weight[0]} bytes')
        if len(hops) == 1:
            return

        for i, _ in enumerate(hops):
            if i == len(hops) - 1:
                break

            nt.add_node(hops[i], label=f'Peer {hops[i]}')
            nt.add_node(hops[i+1], label=f'Peer {hops[i+1]}')
            nt.add_edge(hops[i], hops[i+1])

    with open(message_log, 'r') as fd:
        while True:
            ln = fd.readline()
            if not ln:
                break
            msg = loads(ln.strip())
            process_hops(msg['hops'])

    nt.show_buttons(filter_=['physics', 'interaction', 'layout'])
    nt.save_graph(f'p2p_graph_{self_id}.html')


def main():
    print('Processing p2p chit-chat log ...')
    for (peer, path) in get_file_paths('../../'):
        read_and_plot(peer, abspath(
            join('../..', f'{peer}.traffic.txt')), path)
    print('Generated p2p graphs !')


if __name__ == '__main__':
    main()

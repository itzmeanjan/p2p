#!/usr/bin/python3

from os import walk
from os.path import join, abspath
from re import compile as reg_compile
from typing import List, Tuple
from pandas.core.frame import DataFrame
import pandas


def get_file_paths(root: str) -> List[Tuple[int, str]]:
    reg = reg_compile(r'^((\d{1,})\.traffic\.txt)$')
    paths = []
    for (d, _, files) in walk(root):
        for file in files:
            m = reg.match(file)
            if m:
                paths.append((int(m.group(2)), abspath(join(d, file))))
    return paths


def read(self_id: int, file: str, df: DataFrame) -> DataFrame:
    def is_added(peer_id: int) -> bool:
        yes = False
        for (frm, to, _) in df.values:
            if (frm == self_id and to == peer_id) or (frm == peer_id and to == self_id):
                yes = True
                break
        return yes

    buf = []
    with open(file, 'r') as fd:
        while True:
            ln = fd.readline()
            if not ln:
                break
            vals = [int(i.strip()) for i in ln.split(";")]
            peer_id = vals[0]
            if is_added(peer_id):
                continue
            buf.append({"from": self_id,
                        "to": peer_id,
                       "total": vals[1] + vals[2]})
    df = df.append(buf)
    return df


def read_logs(root: str) -> DataFrame:
    df = pandas.DataFrame()
    for (peer, path) in get_file_paths(root):
        df = read(peer, path, df)
    return df.reset_index(drop=True)


def main():
    print(read_logs("../.."))


if __name__ == '__main__':
    main()

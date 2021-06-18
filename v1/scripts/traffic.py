#!/usr/bin/python3

from os import walk
from os.path import join, abspath
from re import compile as reg_compile
from typing import List, Tuple
from pandas.core.frame import DataFrame
from matplotlib import pyplot as plt
import seaborn as sns


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
    df = DataFrame()
    for (peer, path) in get_file_paths(root):
        df = read(peer, path, df)

    df = df.reset_index(drop=True)
    z = zip(df['from'].values, df['to'].values, df['total'].values)
    return DataFrame([{"pair": f'{i[0]} <-> {i[1]}', "data": i[2]} for i in z])


def plot(df: DataFrame) -> bool:
    try:
        with plt.style.context('dark_background'):
            fig = plt.Figure(figsize=(16, 9), dpi=100)
            sns.barplot(x='pair', y='data', data=df, ax=fig.gca())

            fig.gca().set_xlabel('Peer Connection Pair', labelpad=12)
            fig.gca().set_ylabel('Data Transferred over wire ( in bytes )', labelpad=12)
            fig.suptitle('Total Data over Wire',
                         fontsize=16, y=1)

            fig.savefig(
                'traffic.png',
                bbox_inches='tight',
                pad_inches=.5)
            plt.close(fig)
        return True
    except Exception as e:
        print(f'Error : {e}')
        return False


def main():
    print('Processing traffic log ...')
    if plot(read_logs("../..")):
        print('Generated plot !')
    else:
        print('Failed !')


if __name__ == '__main__':
    main()

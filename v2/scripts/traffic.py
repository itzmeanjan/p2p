#!/usr/bin/python3

from os import walk
from os.path import join, abspath
from re import compile as reg_compile
from typing import List
from pandas.core.frame import DataFrame
from matplotlib import pyplot as plt
import seaborn as sns


def get_file_paths(root: str) -> List[str]:
    reg = reg_compile(r'^((\d{1,})\.traffic\.csv)$')
    paths = []
    for (d, _, files) in walk(root):
        for file in files:
            m = reg.match(file)
            if m:
                paths.append(abspath(join(d, file)))
    return paths


def read(file: str, df: DataFrame) -> DataFrame:
    def is_added(self_id: int, peer_id: int) -> bool:
        yes = False
        for (frm, to, _) in df.values:
            if (frm == self_id and to == peer_id) or (frm == peer_id and to == self_id):
                yes = True
                break
        return yes

    def num(n: str):
        try:
            return int(n)
        except ValueError:
            return int(float(n))

    buf = []
    with open(file, 'r') as fd:
        while True:
            ln = fd.readline()
            if not ln:
                break
            [self_id, peer_id, total] = [
                num(i.strip()) for i in ln.split(";")]
            if total < 1 or is_added(self_id, peer_id):
                continue
            buf.append({"from": self_id,
                        "to": peer_id,
                       "total": total})
    df = df.append(buf)
    return df


def read_all(root: str) -> DataFrame:
    df = DataFrame()
    for p in get_file_paths(root):
        df = read(p, df)

    df = df.reset_index(drop=True)
    z = zip(df['from'].values, df['to'].values, df['total'].values)
    return DataFrame([{"edge": f'{i[0]} <-> {i[1]}', "data": i[2]} for i in z])


def plot(df: DataFrame) -> bool:
    try:
        with plt.style.context('dark_background'):
            fig = plt.Figure(figsize=(16, 9), dpi=100)
            g = sns.barplot(x='edge', y='data', data=df, ax=fig.gca())
            g.set(xticklabels=[])
            g.set(xlabel=None)
            g.tick_params(bottom=False)
            fig.gca().set_ylabel('Data over Wire ( in bytes )', labelpad=12)
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
    if plot(read_all("../..")):
        print('Generated plot !')
    else:
        print('Failed !')


if __name__ == '__main__':
    main()

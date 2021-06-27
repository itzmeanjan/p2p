#!/usr/bin/python3

from os import walk
from os.path import join, abspath
from re import compile as reg_compile
from typing import List, Tuple
from pandas.core.frame import DataFrame
from matplotlib import pyplot as plt
import seaborn as sns


def get_file_paths(root: str) -> List[str]:
    reg = reg_compile(r'^((\d{1,}_\d{1,})\.cost\.csv)$')
    paths = []
    for (d, _, files) in walk(root):
        for file in files:
            m = reg.match(file)
            if m:
                paths.append(abspath(join(d, file)))
    return paths


def read(file: str, df: DataFrame) -> DataFrame:
    def is_added(self_id: int, peer_id: int) -> Tuple[bool, int]:
        yes = False
        idx = 0
        for (_idx, (frm, to, _, _, _)) in enumerate(df.values):
            if (frm == self_id and to == peer_id) or (frm == peer_id and to == self_id):
                yes = True
                idx = _idx
                break
        return yes, idx

    def update_df(idx: int, add_cost: int, del_cost: int, probe_cost: int):
        df.at[idx, 'add_cost'] += add_cost
        df.at[idx, 'del_cost'] += del_cost
        df.at[idx, 'probe_cost'] += probe_cost

    buf = []
    with open(file, 'r') as fd:
        while True:
            ln = fd.readline()
            if not ln:
                break
            [self_id, peer_id, add_cost, del_cost, probe_cost] = [
                int(i.strip()) for i in ln.split(";")]
            yes, idx = is_added(self_id, peer_id)
            if yes:
                continue
            buf.append({"from": self_id,
                        "to": peer_id,
                       "add_cost": add_cost,
                        "del_cost": del_cost,
                        "probe_cost": probe_cost})
    df = df.append(buf)
    return df


def read_all(root: str) -> DataFrame:
    df = DataFrame()
    for p in get_file_paths(root):
        df = read(p, df)
        df = df.reset_index(drop=True)

    z = zip(df['from'].values, df['to'].values, df['add_cost'].values,
            df['del_cost'].values, df['probe_cost'].values)
    return DataFrame([{"edge": f'{i[0]} <-> {i[1]}', "add_cost": i[2], "del_cost": i[3], "probe_cost": i[4]} for i in z])


def plot(df: DataFrame, kind: str, title: str) -> bool:
    try:
        with plt.style.context('dark_background'):
            fig = plt.Figure(figsize=(16, 9), dpi=100)
            g = sns.barplot(x='edge', y=kind, data=df, ax=fig.gca())
            g.set(xticklabels=[])
            g.set(xlabel=None)
            g.tick_params(bottom=False)
            fig.gca().set_ylabel('Data over Wire ( in bytes )', labelpad=12)
            fig.suptitle(title,
                         fontsize=16, y=1)

            fig.savefig(
                f'{kind}_v2.png',
                bbox_inches='tight',
                pad_inches=.5)
            plt.close(fig)
        return True
    except Exception as e:
        print(f'Error : {e}')
        return False


def main():
    print('Processing traffic log ...')
    df = read_all("../..")
    for (kind, title) in [('add_cost', 'Cost to Communicate Edge Addition'),
                          ('del_cost', 'Cost to Communicate Edge Deletion'),
                          ('probe_cost', 'Cost to Communicate Probing Request')]:
        if plot(df, kind, title):
            print(f'[{kind}] Generated plot !')
        else:
            print(f'[{kind}] Failed !')
            return


if __name__ == '__main__':
    main()

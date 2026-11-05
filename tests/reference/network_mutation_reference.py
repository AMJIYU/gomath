"""Development-only actual pinned Manim add/remove oracle; no renderer needed.

This calls the reference implementation, rather than translating its methods.
Ordinary Go tests use only the static JSON. The Python checkout stays read-only.
"""
import sys
sys.dont_write_bytecode = True
from pathlib import Path
import hashlib
import json

root = Path(__file__).resolve().parents[2]
reference = root.parent / "manim"
sys.path.insert(0, str(reference))
import manim
import numpy as np
from manim import Graph, DiGraph

assert Path(manim.__file__).resolve().is_relative_to(reference.resolve())
positions = {"a": [-2., 0., 0.], "b": [2., 0., 0.], "c": [2., 2., 0.], "d": [-2., 2., 0.], "e": [0., 4., 0.]}
cases = []
for cls in (Graph, DiGraph):
    g = cls(["a", "b", "c"], [("a", "b")], layout={v: positions[v] for v in ("a", "b", "c")}, vertex_config={"radius": .12}, edge_config={"buff": 0. if cls is DiGraph else .12})
    original_root = id(g)
    original_nodes = dict(g.vertices)
    original_edges = dict(g.edges)
    steps = []

    def snapshot(operation):
        alive = {**{id(v): "vertex:"+k for k, v in g.vertices.items()}, **{id(v): "edge:"+a+"->"+b for (a, b), v in g.edges.items()}}
        steps.append(dict(operation=operation,
            vertices=list(g.vertices), edges=[list(e) for e in g.edges],
            positions={k: v.get_center().tolist() for k, v in g.vertices.items()},
            endpoints=[dict(edge=list(k), start=v.get_start().tolist(), end=v.get_end().tolist(), has_tip=v.has_tip()) for k, v in g.edges.items()],
            neighbors={k: [b if a==k else a for a,b in g.edges if a==k or (cls is Graph and b==k)] for k in g.vertices},
            root_identity_preserved=id(g)==original_root,
            surviving_node_identity=all(g.vertices[k] is v for k,v in original_nodes.items() if k in g.vertices),
            surviving_edge_identity=all(g.edges[k] is v for k,v in original_edges.items() if k in g.edges),
            root_children=[alive.get(id(v), "unknown") for v in g.submobjects],
            layout_keys=list(g._layout), vertex_config_keys=list(g._vertex_config),
            edge_config_keys=[list(e) for e in g._edge_config]))

    snapshot("initial")
    g.add_vertices("d", "e", positions={v:positions[v] for v in ("d","e")})
    snapshot("add_vertices")
    # This explicit constant buff matches Go bbox clipping only on these axis-
    # aligned connections. No claim about arbitrary shapes/directions follows.
    g.add_edges(("b","c"),("c","d"),("d","a"), edge_config={"buff": .12})
    snapshot("add_edges")
    g.remove_edges(("c","d"))
    snapshot("remove_edges")
    removed = g.remove_vertices("b", "e")
    snapshot("remove_vertices")
    removed_absent = all(v not in g.submobjects for v in removed.submobjects)
    cases.append(dict(kind=cls.__name__, steps=steps, removed_absent=removed_absent,
        notes="Pinned generic DiGraph.add_edges creates Line instances without adding tips. Go deliberately keeps its original directed-arrow policy for newly added links." if cls is DiGraph else "Explicit Line buff=.12 matches bbox clipping for these horizontal/vertical center pairs."))

fixture = dict(reference_commit="16ea443e2a5f1d79db0a721161411ce916d6e733",
    source="manim/mobject/graph.py", source_sha256=hashlib.sha256((reference/"manim/mobject/graph.py").read_bytes()).hexdigest(),
    manim_version=manim.__version__, numpy_version=np.__version__, vertex_radius=.12,
    differences=["Go transactions reject unknown/duplicate endpoints atomically; source can partially apply or auto-create endpoints.", "Go preserves edge-first root child ordering; Manim appends submobjects in method call order.", "Go has no background updater or implicit mutation animation.", "Go newly added directed links always have arrowheads."], cases=cases)
target=root/"tests/data/network/mutation_manim.json"
target.write_text(json.dumps(fixture,indent=2)+"\n")
print(target)

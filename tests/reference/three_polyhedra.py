"""Development-only pinned Manim coordinate and independent support-plane oracle.

No Manim/NumPy/native renderer is imported. A strict arithmetic AST evaluator
extracts the four source constructors; exhaustive supporting triples discover
their convex facets independently of upstream/Go face lists. Output is frozen
JSON consumed by pure-Go tests, not a rendering fallback.
"""
import ast
import hashlib
import itertools
import json
import math
from pathlib import Path
import subprocess

PIN = "16ea443e2a5f1d79db0a721161411ce916d6e733"
ROOT = Path(__file__).resolve().parents[2]
REF = ROOT.parent / "manim"
SOURCE = REF / "manim/mobject/three_d/polyhedra.py"
if subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=REF, text=True).strip() != PIN:
    raise SystemExit("reference checkout is not the pinned commit")
raw = SOURCE.read_bytes()
committed = subprocess.check_output(["git", "show", f"{PIN}:manim/mobject/three_d/polyhedra.py"], cwd=REF)
if raw != committed:
    raise SystemExit("reference source differs from its pinned committed contents")


def expr(node, values):
    if isinstance(node, ast.Constant) and isinstance(node.value, (int, float)):
        return node.value
    if isinstance(node, ast.Name) and node.id in values:
        return values[node.id]
    if isinstance(node, (ast.List, ast.Tuple)):
        return [expr(x, values) for x in node.elts]
    if isinstance(node, ast.UnaryOp) and isinstance(node.op, ast.USub):
        return -expr(node.operand, values)
    if isinstance(node, ast.BinOp):
        a, b = expr(node.left, values), expr(node.right, values)
        if isinstance(node.op, ast.Add): return a+b
        if isinstance(node.op, ast.Mult): return a*b
        if isinstance(node.op, ast.Div): return a/b
    if isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute):
        if isinstance(node.func.value, ast.Name) and node.func.value.id == "np":
            if node.func.attr == "sqrt": return math.sqrt(expr(node.args[0], values))
            if node.func.attr == "array": return expr(node.args[0], values)
    raise ValueError(ast.dump(node))


def sub(a,b): return [x-y for x,y in zip(a,b)]
def dot(a,b): return sum(x*y for x,y in zip(a,b))
def cross(a,b): return [a[1]*b[2]-a[2]*b[1], a[2]*b[0]-a[0]*b[2], a[0]*b[1]-a[1]*b[0]]


def planes(vertices):
    facets = {}
    for i,j,k in itertools.combinations(range(len(vertices)),3):
        a,b,c = vertices[i],vertices[j],vertices[k]
        n = cross(sub(b,a),sub(c,a))
        length = math.sqrt(dot(n,n))
        if length < 1e-10: continue
        n = [x/length for x in n]
        offsets = [dot(n,sub(p,a)) for p in vertices]
        if max(offsets) < 1e-10: pass
        elif min(offsets) > -1e-10: n = [-x for x in n]
        else: continue
        d = dot(n,a)
        on = tuple(i for i,p in enumerate(vertices) if abs(dot(n,p)-d)<1e-10)
        facets.setdefault(on,dict(vertices=on,normal=n,offset=d))
    return list(facets.values())


shapes = []
for cls in ast.parse(raw).body:
    if not isinstance(cls,ast.ClassDef) or cls.name not in ("Tetrahedron","Octahedron","Icosahedron","Dodecahedron"): continue
    init = next(n for n in cls.body if isinstance(n,ast.FunctionDef) and n.name=="__init__")
    values = {"edge_length":1.0}
    for node in init.body:
        if isinstance(node,ast.Assign): values[node.targets[0].id] = expr(node.value,values)
        elif isinstance(node,ast.Expr) and isinstance(node.value,ast.Call):
            params = {kw.arg:kw.value for kw in node.value.keywords if kw.arg is not None}
            vertices,faces = expr(params["vertex_coords"],values),expr(params["faces_list"],values)
    facets = planes(vertices)
    assert {tuple(sorted(f)) for f in faces} == {tuple(p["vertices"]) for p in facets}
    outward, reversed_faces = [],[]
    for i,f in enumerate(faces):
        plane = next(p for p in facets if set(p["vertices"])==set(f))
        a,b,c = [vertices[j] for j in f[:3]]
        if dot(cross(sub(b,a),sub(c,a)),plane["normal"]) < 0:
            f = [f[0]]+list(reversed(f[1:]))
            reversed_faces.append(i)
        outward.append(f)
    shapes.append(dict(name=cls.name,vertices=vertices,source_faces=faces,faces=outward,
                       reversed_faces=reversed_faces,planes=facets))

assert len(shapes)==4
out = ROOT/"tests/data/three/polyhedra"
out.mkdir(parents=True,exist_ok=True)
data = dict(commit=PIN,source="manim/mobject/three_d/polyhedra.py",source_sha256=hashlib.sha256(raw).hexdigest(),
            oracle="whitelist AST arithmetic and independent exhaustive supporting triples",shapes=shapes)
(out/"manim-controls.json").write_text(json.dumps(data,indent=2)+"\n")
for shape in shapes:
    print(shape["name"],len(shape["vertices"]),len(shape["planes"]),"reverse source faces",shape["reversed_faces"])

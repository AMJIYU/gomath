"""Independent development-only curved-network geometry oracle.

NumPy polynomial roots clip circle-tangent cubics against bbox faces. SciPy
quad and brentq independently locate the physical-length arrowhead base.
No Go code is translated/copied, and ordinary Go tests read only the fixture.
Run: python3 tests/reference/network_curved_reference.py
"""
from pathlib import Path
import json, math
import numpy as np
import scipy
from scipy.integrate import quad
from scipy.optimize import brentq

TIP, WIDTH, VERTEX = .16, .08, .12

def circle_cubics(start, end, sweep):
    start,end=np.array(start,float),np.array(end,float)
    d=end-start
    if sweep==0:
        return [np.array([start,start+d/3,start+2*d/3,end])]
    length=np.linalg.norm(d)
    left=np.array([-d[1],d[0]])/length
    center=(start+end)/2+left*(length/(2*math.tan(sweep/2)))
    radius=np.linalg.norm(start-center)
    angle=math.atan2(*(start-center)[::-1])
    step=sweep/8
    out=[]
    for i in range(8):
        a,b=angle+i*step,angle+(i+1)*step
        p=center+radius*np.array([math.cos(a),math.sin(a)])
        q=center+radius*np.array([math.cos(b),math.sin(b)])
        if i==0:p=start.copy()
        if i==7:q=end.copy()
        k=4/3*math.tan(step/4)*radius
        out.append(np.array([p,p+k*np.array([-math.sin(a),math.cos(a)]),q-k*np.array([-math.sin(b),math.cos(b)]),q]))
    return out

def point(c,t):return (1-t)**3*c[0]+3*(1-t)**2*t*c[1]+3*(1-t)*t*t*c[2]+t**3*c[3]
def speed(c,t):return np.linalg.norm(3*((1-t)**2*(c[1]-c[0])+2*(1-t)*t*(c[2]-c[1])+t*t*(c[3]-c[2])))
def length(c,a,b):return quad(lambda t:speed(c,t),a,b,epsabs=1e-12,epsrel=1e-12)[0]
def bbox(p):return [np.array(p)-VERTEX,np.array(p)+VERTEX]
def inside(p,bb):return all(p>bb[0]) and all(p<bb[1])
def sample(curves,t):
    i=min(int(t),len(curves)-1)
    return point(curves[i],t-i)
def clip(curves,boxes):
    roots=[0.,float(len(curves))]
    for i,c in enumerate(curves):
        for bb in boxes:
            for axis in (0,1):
                for face in (bb[0][axis],bb[1][axis]):
                    v=c[:,axis]-face
                    coefficients=[-v[0]+3*v[1]-3*v[2]+v[3],3*v[0]-6*v[1]+3*v[2],3*(v[1]-v[0]),v[0]]
                    for t in np.roots(np.trim_zeros(coefficients,'f')):
                        if abs(t.imag)<1e-9 and 0<=t.real<=1:
                            roots.append(float(i+t.real))
    roots=sorted(roots)
    exposed=[]
    for a,b in zip(roots,roots[1:]):
        if b-a<1e-12:continue
        p=sample(curves,(a+b)/2)
        if not any(inside(p,box) for box in boxes):exposed.append((a,b))
    return exposed[0][0],exposed[-1][1]
def finish(curves,lo,hi):
    segments=[]
    for i,c in enumerate(curves):
        a,b=max(0.,lo-i),min(1.,hi-i)
        if b-a>1e-12:segments.append([c,a,b,length(c,a,b)])
    remaining=sum(s[3] for s in segments)-TIP
    assert remaining>0
    output=[]
    base=None
    for c,a,b,span in segments:
        if remaining<=0:break
        if remaining<span:
            b=brentq(lambda t:length(c,a,t)-remaining,a,b,xtol=1e-14)
        output.append([point(c,a+(b-a)*t).tolist() for t in (0,.25,.5,.75,1)])
        base=point(c,b)
        remaining-=span
    tip=sample(curves,hi)
    u=(tip-base)/np.linalg.norm(tip-base)
    v=np.array([-u[1],u[0]])*WIDTH/2
    return dict(shaft=output,head=[tip.tolist(),(base+v).tolist(),(base-v).tolist()])

cases=[]
for a,b,sweep in [([-2,0],[2,0],0),([-2,0],[2,0],math.pi/2),([-2,0],[2,0],-math.pi/2),([-2,0],[2,0],math.pi),([-1.5,-.4],[1.7,.8],.9),([-.8,.5],[1.5,-.9],-.75)]:
    curves=circle_cubics(a,b,sweep)
    lo,hi=clip(curves,[bbox(a),bbox(b)])
    cases.append(dict(kind='edge',a=a,b=b,sweep=sweep,**finish(curves,lo,hi)))
for side,normal in [('top',[0,1]),('right',[1,0]),('bottom',[0,-1]),('left',[-1,0])]:
    for clockwise in (False,True):
        radius=.35
        n=np.array(normal);s=np.array([n[1],-n[0]])
        center=n*VERTEX
        spread=min(VERTEX*.75,radius*.5)
        start,end=center+s*spread,center-s*spread
        sweep=2*math.pi-2*math.asin(spread/radius)
        if clockwise:start,end,sweep=end,start,-sweep
        curves=circle_cubics(start,end,sweep)
        cases.append(dict(kind='loop',a=[0,0],side=side,radius=radius,clockwise=clockwise,**finish(curves,0.,8.)))
root=Path(__file__).resolve().parents[1]/'data'/'network'
(root/'curved_math.json').write_text(json.dumps(dict(oracle='independent circle/tangent cubics, NumPy polynomial roots, SciPy quad/brentq',numpy=np.__version__,scipy=scipy.__version__,tip_length=TIP,tip_width=WIDTH,vertex_radius=VERTEX,cases=cases),indent=2)+'\n')

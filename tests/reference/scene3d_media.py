"""Independent decoder check for the real 3D timeline -> MP4 pipeline.

Build bin/gomath with CGO_ENABLED=0 -tags noasm first. The output directory must
be new. FFmpeg and Pillow are development references, never Go runtime fallbacks.
"""
import argparse
import hashlib
import json
import math
from pathlib import Path
import subprocess

from PIL import Image

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument("output",type=Path)
parser.add_argument("--scene",choices=["scene3d-timeline","point-morph","mesh-morph","quaternion","oriented-camera","point-lights","platonic-solids","segments3d","checkerboard-surface"],default="scene3d-timeline")
parser.add_argument("--qp",type=int,choices=range(52),default=24,metavar="0..51",help="explicit encoder quality; comparison thresholds are unchanged")
parser.add_argument("--width",type=int,default=320)
parser.add_argument("--height",type=int,default=180)
options=parser.parse_args()
if not(2<=options.width<=4096 and 2<=options.height<=4096) or options.width%2 or options.height%2:
    parser.error("video dimensions must be even and between2 and4096")
root=Path(__file__).resolve().parents[2]
out=options.output.resolve()
out.mkdir(parents=True,exist_ok=False)
binary=root/"bin/gomath"
def source_hashes():
    return {str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest()
            for p in sorted(root.rglob("*.go"))
            if p.relative_to(root).parts[0] not in {"tests","artifacts","bin",".git"}
            and not any(part.startswith("_") for part in p.relative_to(root).parts)}
sources_before=source_hashes()
binary_before=hashlib.sha256(binary.read_bytes()).hexdigest()
commands=[]

def run(args):
    return subprocess.check_output([str(x) for x in args],cwd=root)

args=[binary,"demo","-scene",options.scene,"-format","mp4",
      "-width",str(options.width),"-height",str(options.height),"-fps","8","-codec","predictive",
      "-gop","8","-qp",str(options.qp),"-inter-residual","-motion-radius","1","-out",out/"timeline.mp4"]
run(args);commands.append([str(x) for x in args[1:]])
strict=["ffmpeg","-hide_banner","-loglevel","error","-xerror","-err_detect","explode","-threads","1"]
decoded=run(strict+["-i",out/"timeline.mp4","-map","0:v:0","-f","rawvideo","-pix_fmt","rgb24","-"])
frame_bytes=options.width*options.height*3
assert len(decoded)==40*frame_bytes
info=json.loads(run(["ffprobe","-v","error","-show_streams","-show_format","-of","json",out/"timeline.mp4"]))
assert len(info["streams"])==1
video=info["streams"][0]
assert video["codec_name"]=="h264" and video["width"]==options.width and video["height"]==options.height
assert int(video["nb_frames"])==40 and video["duration"]=="5.000000"
frames=json.loads(run(["ffprobe","-v","error","-select_streams","v:0","-show_entries",
                      "frame=key_frame,best_effort_timestamp_time","-of","json",out/"timeline.mp4"]))["frames"]
keys=[i for i,f in enumerate(frames) if f["key_frame"]]
assert keys==[0,8,16,24,32] and len(frames)==40
for i,frame in enumerate(frames):
    assert abs(float(frame["best_effort_timestamp_time"])-i/8)<1e-6
hashes=[hashlib.sha256(decoded[i*frame_bytes:(i+1)*frame_bytes]).hexdigest() for i in range(40)]
assert len(set(hashes))>=32,"timeline unexpectedly frozen or repeated"

checks=[]
for index in [0,6,10,20,25,30,39]:
    path=out/f"native-{index:02d}.png"
    args=[binary,"demo","-scene",options.scene,"-format","png","-width",str(options.width),"-height",str(options.height),
          "-time",str(index/8),"-out",path]
    run(args);commands.append([str(x) for x in args[1:]])
    reference=Image.open(path).convert("RGB").tobytes()
    actual=decoded[index*frame_bytes:(index+1)*frame_bytes]
    mse=sum((a-b)**2 for a,b in zip(reference,actual))/len(actual)
    psnr=10*math.log10(255**2/mse) if mse else 999
    mae=sum(abs(a-b) for a,b in zip(reference,actual))/len(actual)
    assert psnr>=28 and mae<5,(index,psnr,mae)
    checks.append({"frame":index,"time":index/8,"RGB_MAE":mae,"RGB_PSNR_dB":psnr})

assert source_hashes()==sources_before,"production sources changed during verification"
assert hashlib.sha256(binary.read_bytes()).hexdigest()==binary_before,"binary changed during verification"
result={"scene":options.scene,"generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "binary_sha256":binary_before,
        "go":run(["go","version"]).decode().strip(),
        "decoder":run(["ffmpeg","-version"]).decode().splitlines()[0],
        "commands":commands,"width":options.width,"height":options.height,"qp":options.qp,"frames":40,"keyframes_zero_based":keys,"fps":8,"duration":5,
        "decoded_rgb_sha256":hashlib.sha256(decoded).hexdigest(),"distinct_decoded_frames":len(set(hashes)),
        "native_png_checks":checks,
        "sources_sha256":sources_before,
        "files":{p.name:{"bytes":p.stat().st_size,"sha256":hashlib.sha256(p.read_bytes()).hexdigest()} for p in sorted(out.iterdir()) if p.is_file()},
        "limits":f"One selected five-second 3D timeline, 480x270 2xSSAA 3D raster resampled to{options.width}x{options.height}, static screen-space vector title; predictive QP{options.qp}/GOP8/radius1. Seven RGB comparisons include YUV420 and lossy quantization. Native PNGs share Go scene authoring, so these are container/decoder/timeline integration checks, not an independent 3D geometry oracle or performance/Manim parity measurement."}
with (out/"provenance.json").open("x") as stream:
    json.dump(result,stream,indent=2);stream.write("\n")
print(json.dumps({k:result[k] for k in ["frames","keyframes_zero_based","distinct_decoded_frames","native_png_checks"]},indent=2))

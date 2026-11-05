"""Development-only independent verification of integrated Go media exports.

Build bin/gomath with CGO_ENABLED=0 -tags noasm first. Invoke with a fresh
artifact directory; this script never overwrites outputs. FFmpeg/ffprobe are
independent decoders, not rendering/encoding dependencies of the Go binary.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("output", type=Path)
parser.add_argument("--inter-residual", action="store_true")
parser.add_argument("--motion-radius",type=int,default=0,choices=range(5))
options = parser.parse_args()
assert not options.motion_radius or options.inter_residual,"motion search requires --inter-residual"
root = Path(__file__).resolve().parents[2]
out = options.output.resolve()
out.mkdir(parents=True, exist_ok=False)
binary = root / "bin" / "gomath"

def run(args):
    return subprocess.check_output([str(x) for x in args], cwd=root)

commands=[]
for kind in ["mov", "wav", "flac", "srt", "vtt"]:
    args=[binary,"demo","-scene","composition","-format",kind,"-out",out/("composition."+kind)]
    if kind=="mov":
        args += ["-codec","predictive","-gop","12","-mode-search","-width","480","-height","270","-fps","12"]
        if options.inter_residual:
            args += ["-inter-residual"]
        if options.motion_radius:
            args += ["-motion-radius",str(options.motion_radius)]
    run(args)
    commands.append([str(x) for x in args[1:]])

strict=["ffmpeg","-hide_banner","-loglevel","error","-xerror","-err_detect","explode","-threads","1"]
run(strict+["-i",out/"composition.mov","-f","null","-"])
streams=json.loads(run(["ffprobe","-v","error","-show_streams","-show_format","-of","json",out/"composition.mov"]))
video=next(s for s in streams["streams"] if s["codec_type"]=="video")
audio=next(s for s in streams["streams"] if s["codec_type"]=="audio")
assert int(video["nb_frames"])==60 and video["width"]==480 and video["height"]==270
assert audio["codec_name"]=="pcm_s16le" and int(audio["sample_rate"])==48000 and audio["channels"]==2
assert video["duration"]=="5.000000" and audio["duration"]=="5.000000"
frames=json.loads(run(["ffprobe","-v","error","-select_streams","v:0","-show_entries","frame=key_frame,best_effort_timestamp_time","-of","json",out/"composition.mov"]))["frames"]
keys=[i for i,f in enumerate(frames) if f["key_frame"]]
assert keys==[0,12,24,36,48]
assert len(frames)==60
for i,f in enumerate(frames):
    assert abs(float(f["best_effort_timestamp_time"])-i/12)<1e-6

pcm=[]
for kind in ["mov","wav","flac"]:
    decoded=run(strict+["-i",out/("composition."+kind),"-map","0:a:0","-c:a","pcm_s16le","-f","s16le","-"])
    pcm.append(decoded)
assert pcm[0]==pcm[1]==pcm[2] and len(pcm[0])==240000*2*2
# Native Go FLAC import into a shorter scene and WAV export. Soundtrack's public
# contract preserves audio tails beyond the visual timeline, so the complete
# original must survive unchanged. This catches dropping, shifting or truncation.
args=[binary,"demo","-scene","reference","-format","wav","-audio",out/"composition.flac",
      "-out",out/"input-roundtrip.wav"]
run(args)
commands.append([str(x) for x in args[1:]])
roundtrip=run(strict+["-i",out/"input-roundtrip.wav","-map","0:a:0","-c:a","pcm_s16le","-f","s16le","-"])
assert roundtrip==pcm[0]
for kind,format in [("srt","srt"),("vtt","webvtt")]:
    decoded=run(strict+["-i",out/("composition."+kind),"-map","0:s:0","-f",format,"-"])
    assert "组合动画与音频：纯 Go" in decoded.decode("utf-8")

files={p.name:{"bytes":p.stat().st_size,"sha256":hashlib.sha256(p.read_bytes()).hexdigest()} for p in sorted(out.iterdir()) if p.is_file()}
sources={str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(root.rglob("*.go")) if p.relative_to(root).parts[0] not in {"tests","artifacts","bin",".git"}}
result={"generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "go":run(["go","version"]).decode().strip(),"binary_sha256":hashlib.sha256(binary.read_bytes()).hexdigest(),
        "decoder":run(["ffmpeg","-version"]).decode().splitlines()[0],
        "sources_sha256":sources,"commands":commands,"files":files,"video_frames":60,
        "keyframes_zero_based":keys,"pcm_sample_frames":240000,"pcm_channels":2,"pcm_rate":48000,
        "pcm_decoded_sha256":hashlib.sha256(pcm[0]).hexdigest(),
        "inter_residual":options.inter_residual,
        "motion_radius":options.motion_radius,
        "checks":"strict complete MOV decode; sync frames and presentation timestamps; MOV/WAV/FLAC decoded PCM16 exactly equal; native Go FLAC input roundtrip preserves the full five-second audio tail of a one-second scene; UTF-8 SRT/WebVTT decoded independently",
        "limits":"One five-second synthetic stereo scene and one FLAC import into a one-second scene; no general random seek, motion range/quality, general media or Manim parity claim. No throughput benchmark."}
with (out/"provenance.json").open("x") as f:
    json.dump(result,f,indent=2)
    f.write("\n")
print(json.dumps({k:result[k] for k in ["files","keyframes_zero_based","checks"]},indent=2))

#!/usr/bin/env python3
"""Development-only Microsoft ADPCM oracle and complete independent decodes.
No Go invocation; fixtures consumed by pure Go tests. Requires numpy/soundfile,
FFmpeg. All external output is retained as hashes/counts, without trim options.
"""
import hashlib
import io
import json
import math
from pathlib import Path
import struct
import subprocess
import urllib.request

import numpy as np
import soundfile as sf

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'tests/data/audio/ms_adpcm'
COEFF = [(256, 0), (512, -256), (0, 0), (192, 64), (240, 0), (460, -208), (392, -232)]
ADAPT = [230,230,230,230,307,409,512,614,768,614,512,409,307,230,230,230]

def sha(b): return hashlib.sha256(b).hexdigest()
def run(args, data=None): return subprocess.run(args, input=data, capture_output=True)
def chunk(k, b): return k + struct.pack('<I', len(b)) + b + bytes(len(b) & 1)
def riff(entries):
    body = b'WAVE' + b''.join(chunk(k, b) for k, b in entries)
    return b'RIFF' + struct.pack('<I', len(body)) + body

def parts(w):
    assert w[:4] == b'RIFF' and w[8:12] == b'WAVE'
    assert struct.unpack_from('<I', w, 4)[0] + 8 == len(w)
    p, result = 12, {}
    while p < len(w):
        k, n = struct.unpack_from('<4sI', w, p)
        assert k not in result
        result[k] = w[p+8:p+8+n]
        p += 8+n+(n&1)
    assert p == len(w)
    return result

def wave(ch, align, blocks, count=None, coeff=COEFF, rate=48000):
    spb = 2+(align-7*ch)*2//ch
    fmt = struct.pack('<HHIIHHHHH', 2, ch, rate, rate*align//spb, align, 4, 4+4*len(coeff), spb, len(coeff))
    fmt += b''.join(struct.pack('<hh', *pair) for pair in coeff)
    if count is None: count = len(blocks)*spb
    return riff([(b'fmt ', fmt), (b'fact', struct.pack('<I', count)), (b'data', b''.join(blocks))])

def block(ch, align, pred, delta, first, second, codes):
    b = bytes(pred)+struct.pack('<'+'h'*ch, *delta)+struct.pack('<'+'h'*ch, *first)+struct.pack('<'+'h'*ch, *second)
    return b + bytes(codes[i % len(codes)] for i in range(align-7*ch))

def oracle(w, floor=False, short_delta=False):
    p = parts(w); f = p[b'fmt ']
    tag,ch,rate,avg,align,bits,cb,spb,ncoef = struct.unpack_from('<HHIIHHHHH', f)
    assert tag == 2 and bits == 4 and len(f) == 22+4*ncoef and cb == 4+4*ncoef
    assert spb == 2+(align-7*ch)*2//ch and len(p[b'data']) % align == 0
    coeff = [struct.unpack_from('<hh', f, 22+4*i) for i in range(ncoef)]
    assert coeff[:7] == COEFF
    values = []
    for start in range(0, len(p[b'data']), align):
        b = p[b'data'][start:start+align]
        states = []
        for i in range(ch):
            delta, = struct.unpack_from('<h', b, ch+2*i)
            first, = struct.unpack_from('<h', b, 3*ch+2*i)
            second, = struct.unpack_from('<h', b, 5*ch+2*i)
            assert delta >= 0 and b[i] < len(coeff)
            states.append([first, second, delta, coeff[b[i]]])
        values.extend(s[1] for s in states)
        values.extend(s[0] for s in states)
        for i, code in enumerate(c for v in b[7*ch:] for c in (v >> 4, v & 15)):
            s = states[i % ch]
            numerator = s[0]*s[3][0]+s[1]*s[3][1]
            prediction = numerator//256 if floor else ((abs(numerator)//256)*(-1 if numerator < 0 else 1))
            value = max(-32768, min(32767, prediction + (code-16 if code >= 8 else code)*s[2]))
            next_delta=s[2]*ADAPT[code]//256
            if short_delta: next_delta=(next_delta+32768)%65536-32768
            s[1],s[0],s[2] = s[0],value,max(16,next_delta)
            assert s[2] <= (2**63-1)//768
            values.append(value)
    count, = struct.unpack('<I', p[b'fact'])
    assert len(values)//ch-spb < count <= len(values)//ch or count == len(values) == 0
    return values, ch, rate, count

def pcm(values): return struct.pack('<'+'h'*len(values), *values)
def compare(raw, expected):
    a = np.frombuffer(raw, dtype='<i2').astype(np.int64)
    b = np.array(expected, dtype=np.int64)
    n = min(len(a),len(b))
    return {'WholeBytes':len(raw), 'WholeSHA256':sha(raw), 'ExpectedScalarSamples':len(b),
            'ComparedScalarSamples':n, 'DifferentScalarSamples':int(np.count_nonzero(a[:n]!=b[:n])),
            'MaxAbsDifference':int(np.max(np.abs(a[:n]-b[:n]))) if n else 0,
            'ExtraScalarSamples':max(0,len(a)-len(b)), 'MissingScalarSamples':max(0,len(b)-len(a))}

def external(path, expected):
    ff = run(['ffmpeg','-v','error','-xerror','-err_detect','explode','-i',str(path),'-map','0:a:0','-c:a','pcm_s16le','-f','s16le','pipe:1'])
    f = {'Accepted':ff.returncode==0, 'Stderr':ff.stderr.decode(errors='replace'), **compare(ff.stdout,expected)}
    try:
        data,rate = sf.read(str(path),dtype='int16',always_2d=True)
        snd = {'Accepted':True,'Rate':rate, **compare(data.astype('<i2').tobytes(),expected)}
    except Exception as e: snd={'Accepted':False,'Error':str(e)}
    return f,snd

def main():
    OUT.mkdir(parents=True,exist_ok=True)
    manifest = {'FFmpegVersion':run(['ffmpeg','-version']).stdout.decode().splitlines()[0],
                'LibsndfileVersion':sf.__libsndfile_version__, 'Sources':[], 'Cases':[], 'RejectedCases':[]}
    cached = json.loads((OUT/'manifest.json').read_text()).get('Sources',[]) if (OUT/'manifest.json').exists() else []
    for title,url in [('Microsoft Multimedia Standards Update1994','https://www.mmsp.ece.mcgill.ca/Documents/AudioFormats/WAVE/Docs/RIFFNEW.pdf'),
                      ('FFmpeg8.0.1 decoder','https://raw.githubusercontent.com/FFmpeg/FFmpeg/n8.0.1/libavcodec/adpcm.c'),
                      ('libsndfile1.2.2 decoder','https://raw.githubusercontent.com/libsndfile/libsndfile/1.2.2/src/ms_adpcm.c')]:
        prior=next((v for v in cached if v['URL']==url),None)
        if prior is not None:
            manifest['Sources'].append(prior)  # reuse previously fetched immutable source ledger
        else:
            b=urllib.request.urlopen(url,timeout=30).read()
            manifest['Sources'].append({'Title':title,'URL':url,'SHA256':sha(b)})
    streams=[]
    for ch in (1,2):
        for pred in range(7):
            align=7*ch+64
            b=block(ch,align,[pred]*ch,[31+ch]*ch,[-103,32767][:ch],[17,-32768][:ch],[i*17 for i in range(16)]+list(range(48)))
            streams.append((f'spec_pred{pred}_{ch}ch',wave(ch,align,[b,b]),'all16codes/signed seeds/block reset',False))
        for delta in (0,1,15,16,32767):
            align=7*ch+16
            b=block(ch,align,[3]*ch,[delta]*ch,[-1]*ch,[0]*ch,[0,0x17,0x8f,0x70])
            streams.append((f'spec_delta{delta}_{ch}ch',wave(ch,align,[b]),'literal initial delta, adaptive floor after first nibble',False))
        for align in (7*ch,7*ch+1,255,256,1024):
            b=block(ch,align,[4]*ch,[16]*ch,[-1]*ch,[0]*ch,[0,0x10,0xf0])
            spb=2+(align-7*ch)*2//ch
            streams.append((f'spec_align{align}_{ch}ch',wave(ch,align,[b,b],spb+1),'header-only/odd-block/fact trim',False))
        for count in (7,8,256):
            coeff=COEFF+[(-32768,-32768)]*(count-7)
            align=7*ch+8
            b=block(ch,align,[count-1]*ch,[16]*ch,[-32768]*ch,[-32768]*ch,[0,0x10,0xf0,0x8f])
            streams.append((f'spec_coeff{count}_{ch}ch',wave(ch,align,[b],coeff=coeff),'file coefficient table, int64 intermediate',False))
        for pred in (0,3):
            align=256
            codes=([c*17 for c in range(16) for _ in range(1)] if pred==0 else [0])
            # Spread high-magnitude codes apart so adaptation never overflows
            # libsndfile's short delta. This separates rounding from overflow.
            if pred==0: codes=[v for c in range(16) for v in [c*17]+[0]*15]
            b=block(ch,align,[pred]*ch,[16]*ch,[-1]*ch,[0]*ch,codes)
            streams.append((f'spec_bounded_pred{pred}_{ch}ch',wave(ch,align,[b]),'bounded adaptation and isolated signed prediction rounding',False))
        for align in (256,1024):
            # Independent encoder emits full padded blocks and its own fact policy.
            values=np.array([int(27000*math.sin((f+1)*(.11+c*.027))) for f in range(48000) for c in range(ch)],dtype='<i2')
            path=OUT/f'ffmpeg_{ch}ch_align{align}.wav'
            ff=run(['ffmpeg','-v','error','-f','s16le','-ar','48000','-ac',str(ch),'-i','pipe:0','-map_metadata','-1','-c:a','adpcm_ms','-block_size',str(align),'-y',str(path)],values.tobytes())
            assert ff.returncode==0,ff.stderr
            streams.append((path.stem,path.read_bytes(),'FFmpeg encoder,48000frames signed16 sine',True))
        values=np.array([(-32768,32767,-1,0,1)[i%5] for i in range(4099*ch)],dtype=np.int16).reshape(-1,ch)
        stream=io.BytesIO();sf.write(stream,values,22050,format='WAV',subtype='MS_ADPCM')
        streams.append((f'libsndfile_{ch}ch',stream.getvalue(),'libsndfile encoder,4099frames extrema/zero',False))
    for name,encoded,origin,benchmark in streams:
        values,ch,rate,count=oracle(encoded); floored,*_=oracle(encoded,True)
        path=OUT/(name+'.wav');path.write_bytes(encoded)
        expected=pcm(values[:count*ch]);rawname=name+'.s16le';(OUT/rawname).write_bytes(expected)
        ff,snd=external(path,values[:count*ch])
        # Also compare complete decodes to full-block oracle; no external crop.
        ff_full=run(['ffmpeg','-v','error','-xerror','-err_detect','explode','-i',str(path),'-map','0:a:0','-c:a','pcm_s16le','-f','s16le','pipe:1'])
        ff['FullCapacityComparison']=compare(ff_full.stdout,values)
        try:
            data,_=sf.read(str(path),dtype='int16',always_2d=True)
            snd['FloorOracleComparison']=compare(data.astype('<i2').tobytes(),floored[:count*ch])
            if struct.unpack_from('<H',parts(encoded)[b'fmt '],20)[0]==7:
                wrapped,*_=oracle(encoded,True,True)
                snd['Signed16DeltaFloorComparison']=compare(data.astype('<i2').tobytes(),wrapped)
        except Exception: pass
        manifest['Cases'].append({'File':path.name,'PCMFile':rawname,'EncodedSHA256':sha(encoded),'PCMSHA256':sha(expected),
            'Channels':ch,'Rate':rate,'Frames':count,'Capacity':len(values)//ch,'Origin':origin,'Benchmark':benchmark,
            'DiscardMetadata':b'LIST' in parts(encoded), 'FFmpeg':ff,'Libsndfile':snd})
        print(name,'fact',count,'capacity',len(values)//ch,'FF',ff['Accepted'],ff['DifferentScalarSamples'],'snd',snd.get('DifferentScalarSamples'),flush=True)
    # Explicit subset boundaries, with actual external behavior recorded.
    base=parts(streams[0][1]); data=base[b'data']
    negative=bytearray(data);negative[1:3]=struct.pack('<H',32768)
    badpredictor=bytearray(data);badpredictor[0]=7
    for name,payload,reason in [('negative_delta',bytes(negative),'unsupported'),('short_final_block',data[:-1],'unsupported'),('predictor_outside_table',bytes(badpredictor),'invalid')]:
        encoded=riff([(b'fmt ',base[b'fmt ']),(b'fact',base[b'fact']),(b'data',payload)])
        path=OUT/('reject_'+name+'.wav');path.write_bytes(encoded)
        ff,snd=external(path,[])
        manifest['RejectedCases'].append({'File':path.name,'EncodedSHA256':sha(encoded),'Category':reason,'FFmpeg':ff,'Libsndfile':snd})
    manifest['Summary']={'Cases':len(manifest['Cases']),'ScalarSamples':sum(c['Frames']*c['Channels'] for c in manifest['Cases']),
        'FFmpegExactFact':sum(c['FFmpeg']['Accepted'] and c['FFmpeg']['WholeSHA256']==c['PCMSHA256'] for c in manifest['Cases']),
        'LibsndfileExactFact':sum(c['Libsndfile']['Accepted'] and c['Libsndfile']['WholeSHA256']==c['PCMSHA256'] for c in manifest['Cases'])}
    (OUT/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
    print(manifest['Summary'])

if __name__=='__main__':main()

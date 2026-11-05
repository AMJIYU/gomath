#!/usr/bin/env python3
"""Development-only Sun/NeXT AU spec oracle and complete external references.
Requires FFmpeg, numpy, soundfile/libsndfile. Go tests/runtime never invoke it.
Oracle uses explicit signed integers and already independently verified G.711
midpoint tables, never Go output. No duration/trim arguments hide disagreement.
"""
import hashlib
import io
import json
from pathlib import Path
import struct
import subprocess
import urllib.request

import numpy as np
import soundfile as sf

ROOT=Path(__file__).resolve().parents[2]
OUT=ROOT/'tests/data/audio/au'
TABLE=ROOT/'tests/data/audio/g711/code_tables.json'
ENCODINGS={1:('ULAW',1),2:('PCM_S8',1),3:('PCM_16',2),4:('PCM_24',3),5:('PCM_32',4),27:('ALAW',1)}

def sha(b):return hashlib.sha256(b).hexdigest()
def run(args,data=None):return subprocess.run(args,input=data,capture_output=True)
def au(tag,ch,frames,unknown=False,annotation=b'',rate=48000):
    width=ENCODINGS[tag][1]
    if tag in (1,27):data=bytes((f+chidx*37)&255 for f in range(frames) for chidx in range(ch))
    else:
        scale=1<<(8*width-1)
        vals=[-scale,scale-1,-1,0,1,scale//2,-scale//2]
        data=b''.join(vals[(f*3+c)%len(vals)].to_bytes(width,'big',signed=True) for f in range(frames) for c in range(ch))
    return struct.pack('>4sIIIII',b'.snd',24+len(annotation),0xffffffff if unknown else len(data),tag,rate,ch)+annotation+data

def oracle(w,tables):
    magic,offset,n,tag,rate,ch=struct.unpack_from('>4sIIIII',w)
    assert magic==b'.snd' and (offset==24 or offset>=28)
    width=ENCODINGS[tag][1];data=w[offset:] if n==0xffffffff else w[offset:offset+n]
    assert len(data)%(ch*width)==0 and (n==0xffffffff or len(data)==n)
    if tag in (1,27):v=[tables['MuLaw' if tag==1 else 'ALaw'][b]<<16 for b in data]
    else:v=[int.from_bytes(data[i:i+width],'big',signed=True)<<(32-width*8) for i in range(0,len(data),width)]
    return struct.pack('<'+'i'*len(v),*v),len(v)//ch,rate,ch

def compare(raw,expected):
    n=min(len(raw),len(expected))//4
    a=np.frombuffer(raw[:n*4],dtype='<i4').astype(np.int64);b=np.frombuffer(expected[:n*4],dtype='<i4').astype(np.int64)
    return {'WholeBytes':len(raw),'WholeSHA256':sha(raw),'DifferentScalarSamples':int(np.count_nonzero(a!=b)),
            'MaxAbsDifference':int(np.max(np.abs(a-b))) if n else 0,'ExtraBytes':max(0,len(raw)-len(expected)),'MissingBytes':max(0,len(expected)-len(raw))}

def external(path,expected):
    f=run(['ffmpeg','-v','error','-xerror','-err_detect','explode','-i',str(path),'-map','0:a:0','-c:a','pcm_s32le','-f','s32le','pipe:1'])
    ff={'Accepted':f.returncode==0,'Stderr':f.stderr.decode(errors='replace'),**compare(f.stdout,expected)}
    try:
        values,rate=sf.read(str(path),dtype='int32',always_2d=True)
        snd={'Accepted':True,'Rate':rate,'Channels':values.shape[1],**compare(values.astype('<i4').tobytes(),expected)}
    except Exception as e:snd={'Accepted':False,'Error':str(e)}
    return ff,snd

def main():
    OUT.mkdir(parents=True,exist_ok=True)
    previous=json.loads((OUT/'manifest.json').read_text()).get('Sources',[]) if (OUT/'manifest.json').exists() else []
    m={'FFmpegVersion':run(['ffmpeg','-version']).stdout.decode().splitlines()[0],'LibsndfileVersion':sf.__libsndfile_version__,
       'G711MidpointTableSHA256':sha(TABLE.read_bytes()),'Sources':[],'Cases':[],'RejectedCases':[]}
    for title,url in [('SunOS au(4),15Jan2001','https://docs.oracle.com/cd/E19253-01/816-5174/6mbb98ucf/index.html'),
                      ('libsndfile1.2.2 AU reader','https://raw.githubusercontent.com/libsndfile/libsndfile/1.2.2/src/au.c'),
                      ('FFmpeg8.0.1 AU demuxer','https://raw.githubusercontent.com/FFmpeg/FFmpeg/n8.0.1/libavformat/au.c')]:
        old=next((s for s in previous if s['URL']==url),None)
        if old:m['Sources'].append(old)
        else:
            b=urllib.request.urlopen(url,timeout=20).read()
            m['Sources'].append({'Title':title,'URL':url,'SHA256':sha(b)})
    tables=json.loads(TABLE.read_text());streams=[];rejected=[]
    for tag,(sub,width) in ENCODINGS.items():
        for ch in (1,2,3,6,64):streams.append((f'spec_tag{tag}_{ch}ch',au(tag,ch,256),False,'spec signed endpoints / all G711 codes per channel',False))
        for ch in (1,3):streams.append((f'spec_tag{tag}_{ch}ch_unknown',au(tag,ch,257,True),False,'unknown-size complete EOF',False))
        for frames in (0,1):streams.append((f'spec_tag{tag}_frames{frames}',au(tag,1,frames),False,'empty / one frame',False))
        for ann in (bytes(4),b'title\0\xff\x00',bytes(4101)):
            name='text_binary' if any(ann) else f'zeros{len(ann)}'
            streams.append((f'spec_tag{tag}_annotation_{name}',au(tag,3,17,annotation=ann),any(ann),'explicit info offset including unaligned offset and binary data',False))
        streams.append((f'spec_tag{tag}_boundary',au(tag,3,11003),False,'32KiB scratch crossing / incomplete reader chunks',False))
        for ch in (1,2):
            # Source values are normalized signed32 endpoint patterns, not Go.
            src=np.array([(-2147483648,2147483392,-65536,0,65536)[i%5] for i in range(257*ch)],dtype=np.int32).reshape(-1,ch)
            bio=io.BytesIO();sf.write(bio,src,44100,format='AU',subtype=sub,endian='BIG')
            streams.append((f'libsndfile_tag{tag}_{ch}ch',bio.getvalue(),False,'independent libsndfile AU encoder',False))
        streams.append((f'bench_tag{tag}_known',au(tag,2,48000),False,'48000-frame fixed input for timed decoding',True))
        streams.append((f'bench_tag{tag}_unknown',au(tag,2,48000,True),False,'48000-frame fixed input for timed decoding',True))
    for tag in (2,5):
        for rate in (1,768000):streams.append((f'spec_tag{tag}_rate{rate}',au(tag,3,17,rate=rate),False,'integer sample-rate boundary',False))
    # Actual independently encoded variants outside supported scope.
    for sub in ('FLOAT','DOUBLE','G721_32','G723_24','G723_40'):
        try:
            bio=io.BytesIO();sf.write(bio,np.zeros((64,1),dtype=np.float64),8000,format='AU',subtype=sub,endian='BIG')
            rejected.append((f'libsndfile_unsupported_{sub}',bio.getvalue(),'unsupported'))
        except Exception as e:m.setdefault('UnavailableEncoders',[]).append({'Subtype':sub,'Error':str(e)})
    bio=io.BytesIO();sf.write(bio,np.array([-32768,32767,0],dtype=np.int16),8000,format='AU',subtype='PCM_16',endian='LITTLE')
    rejected.append(('libsndfile_little_endian',bio.getvalue(),'unsupported'))
    for name,w,discard,origin,benchmark in streams:
        expected,frames,rate,ch=oracle(w,tables);path=OUT/(name+'.au');path.write_bytes(w)
        (OUT/(name+'.s32le')).write_bytes(expected);ff,snd=external(path,expected)
        # Never accept an observed numeric difference as an oracle update.
        if ff['Accepted']:assert ff['WholeSHA256']==sha(expected),(name,'FFmpeg complete disagreement',ff)
        if snd['Accepted']:assert snd['WholeSHA256']==sha(expected),(name,'libsndfile complete disagreement',snd)
        m['Cases'].append({'File':path.name,'PCMFile':name+'.s32le','EncodedSHA256':sha(w),'PCMSHA256':sha(expected),
          'Frames':frames,'Rate':rate,'Channels':ch,'Encoding':struct.unpack_from('>I',w,12)[0],
          'AllowUnknownSize':struct.unpack_from('>I',w,8)[0]==0xffffffff,'DiscardMetadata':discard,'Benchmark':benchmark,'Origin':origin,'FFmpeg':ff,'Libsndfile':snd})
        print(name,frames,'frames','FF',ff['Accepted'],'SF',snd['Accepted'],flush=True)
    # Preserve malformed input and report any external permissiveness.
    good=au(3,2,3)
    bad=bytearray(good);struct.pack_into('>I',bad,4,20);rejected.append(('spec_offset_inside_header',bytes(bad),'invalid'))
    bad=bytearray(good);struct.pack_into('>I',bad,8,11);rejected.append(('spec_partial_declared_frame',bytes(bad),'invalid'))
    rejected.append(('spec_short_known_payload',good[:-1],'truncated'))
    bad=bytearray(au(4,3,3,True));bad.pop();rejected.append(('spec_partial_unknown_frame',bytes(bad),'invalid'))
    for name,w,category in rejected:
        path=OUT/(name+'.au');path.write_bytes(w);ff,snd=external(path,b'')
        m['RejectedCases'].append({'File':path.name,'EncodedSHA256':sha(w),'Category':category,'FFmpeg':ff,'Libsndfile':snd})
    m['Summary']={'Cases':len(m['Cases']),'ScalarSamples':sum(c['Frames']*c['Channels'] for c in m['Cases']),
      'FFmpegCompleteExact':sum(c['FFmpeg']['Accepted'] and c['FFmpeg']['WholeSHA256']==c['PCMSHA256'] for c in m['Cases']),
      'LibsndfileCompleteExact':sum(c['Libsndfile']['Accepted'] and c['Libsndfile']['WholeSHA256']==c['PCMSHA256'] for c in m['Cases']),
      'RejectedCases':len(m['RejectedCases'])}
    (OUT/'manifest.json').write_text(json.dumps(m,indent=2)+'\n');print(m['Summary'])
if __name__=='__main__':main()

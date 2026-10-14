#!/usr/bin/env python3
"""Produce a device registry with only this app's three blocks added/replaced."""
import argparse
import re
from pathlib import Path
parser=argparse.ArgumentParser()
parser.add_argument('--current',required=True)
parser.add_argument('--output',required=True)
args=parser.parse_args()
source=Path(args.current).read_text()
extra=(Path(__file__).resolve().parents[1]/'device/voice-functions.conf').read_text()
intro=extra.split('[device_function]',1)[0].strip()
source=source.replace(intro,'')
parts=re.split(r'(?=^\[device_function\]\s*$)',source,flags=re.MULTILINE)
owned={'open_meeting_recording','close_meeting_recording','bind_meeting_account'}
keep=[]
for part in parts:
    match=re.search(r'^name\s*=\s*(\S+)\s*$',part,re.MULTILINE)
    if not match or match.group(1) not in owned:
        keep.append(part.rstrip())
result='\n\n'.join(keep)+'\n\n'+extra
for name in owned:
    if len(re.findall(r'^name='+name+r'\s*$',result,re.MULTILINE))!=1:
        raise SystemExit('Invalid duplicate voice function')
Path(args.output).write_text(result)

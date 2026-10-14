from pathlib import Path
import subprocess
import tempfile
script=Path(__file__).with_name('register-voice.py')
with tempfile.TemporaryDirectory() as directory:
    root=Path(directory)
    source=root/'source.conf';source.write_text('[device_function]\nname=existing_function\nscript_path=/usr/bin/existing.sh\n')
    first=root/'first.conf';second=root/'second.conf'
    for current,output in [(source,first),(first,second)]:
        subprocess.run(['python3',str(script),'--current',str(current),'--output',str(output)],check=True)
    assert first.read_text()==second.read_text()
    assert 'name=existing_function' in first.read_text()
    assert first.read_text().count('name=open_meeting_recording')==1
    assert first.read_text().count('name=close_meeting_recording')==1
print('PASS: voice registry retains other functions and is idempotent')

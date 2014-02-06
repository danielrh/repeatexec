import os
import subprocess
import sys
import threading
subprocess.call(["go","build"])
try:
    os.mkfifo("/tmp/stdin",0666)
except OSError:
    pass
try:
    os.mkfifo("/tmp/stdout",0666)
except OSError:
    pass
try:
    os.mkfifo("/tmp/stderr",0666)
except OSError:
    pass
os.chmod("/tmp/stdin",0660)
os.chmod("/tmp/stdout",0660)
os.chmod("/tmp/stderr",0660)
def echo_stderr():
    while True:
        with open('/tmp/stderr','r') as stderr:            
            sys.stderr.write(stderr.readline())
thread = threading.Thread(target=echo_stderr)
thread.setDaemon(True)
thread.start()
repeatexec = subprocess.Popen(["sudo","./repeatexec"],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=sys.stderr)
with open("example_commands.json") as example:
    for line in example.readlines():
        if len(line.strip()):
            print "EXECUTING ",line
            repeatexec.stdin.write(line)
            repeatexec.stdin.flush()
            with open('/tmp/stdin','w') as stdin:
                pass
            with open('/tmp/stdout','r') as stdout:
                print stdout.read()
            exitcode = repeatexec.stdout.read(1)
            print "RESPONSE ", ord(exitcode) if len(exitcode) else 'END OF TEST: SUCCESS'

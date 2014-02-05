go build
mkfifo /tmp/stdin
mkfifo /tmp/stdout
mkfifo /tmp/stderr
chmod ug+rw /tmp/stdin
chmod ug+rw /tmp/stdout
chmod ug+rw /tmp/stderr
sudo ./repeatexec < example_commands.json &
echo "" >> /tmp/stdin
cat /tmp/stderr &
cat /tmp/stdout

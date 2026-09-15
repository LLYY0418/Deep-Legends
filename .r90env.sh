mkdir -p /dev/shm/r90tmp
D=/sessions/youthful-determined-volta/mnt/deep-legends
O=/sessions/youthful-determined-volta/mnt/outputs
export PATH="$O/.gotoolchain/bin:$PATH"
export GOROOT="$O/.gotoolchain"
export GOCACHE="$D/.gocache"
export GOMODCACHE="$D/.gomodcache"
export TMPDIR=/dev/shm/r90tmp
export GOTMPDIR=/dev/shm/r90tmp
cd "$D"

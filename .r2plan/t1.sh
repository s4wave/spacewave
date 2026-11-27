#!/bin/bash
C=/media/storage-2/cjs-goalc/gocache; mkdir -p $C/build $C/tmp
export TMPDIR=$C/tmp GOCACHE=$C/build GOMODCACHE=$C/mod GOPATH=$C/gopath GOTMPDIR=$C/tmp
export GOPRIVATE=github.com/s4wave GOPROXY=off GOFLAGS=-mod=mod GOTOOLCHAIN=local
m=$(free -m | awk '/^Mem:/{print $7}'); d=$(df -m --output=avail /media/storage-2 | tail -1 | tr -d ' ')
echo "gate mem_avail=$m disk=$d"; [ "$m" -ge 6000 ] && [ "$d" -ge 6000 ] || { echo GATE-FAIL; exit 2; }
[ "$(pgrep -c -f 'go (test|build)')" = 0 ] || { echo GATE-BUSY; exit 2; }
cd /media/storage-2/cjs-goalc/r2plan/src || exit 1
which go; go version
s=$(date +%s)
timeout 600 nice -n 19 go test -v -p 1 -count=1 -run 'TestOrderPacksByTime|TestRescueKeepSet|TestKeeperOnceAndVerify|TestOrderPacks$|TestRescueWhole$|TestRescueSharedNum' ./cmd/spacewave-world-rescue/ 2>&1 | tail -30
echo "rc=${PIPESTATUS[0]} $(( $(date +%s) - s ))s"

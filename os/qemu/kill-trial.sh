#!/bin/bash
# kill-trial.sh kills the console guest twice during writes and checks, after
# each reopen, that every record the guest acknowledged after fsync survived.
# Run it on a KVM host as a user in the kvm group, from a fresh trial dir.
# Usage: kill-trial.sh DIR   (DIR holds vmlinuz initramfs.cpio.gz btrfs.img run-console.sh)
# The disk must be large enough to clear the data Volume's disk reserve.
set -euo pipefail
D=$(cd "$1" && pwd)
S=oskill-$$
H=$D/host.log
cd "$D"
note() { echo "$(date -u +%FT%T.%NZ) $*" | tee -a "$H"; }
waitfor() { # file pattern seconds
	local i
	for ((i = 0; i < $3 * 20; i++)); do
		if tr -d '\r' < "$1" 2>/dev/null | grep -q -E -- "$2"; then return 0; fi
		sleep 0.05
	done
	note "TIMEOUT waiting for /$2/ in $1"; return 1
}
qpid() { pgrep -u "$(id -u)" -f "qemu-system-x86_64 .*file=$D/btrfs.img" | head -1; }
send() { tmux load-buffer -b p - <<<"$1"; tmux paste-buffer -b p -t "$S" -d; }
boot() { # logname
	LOG=$D/$1
	tmux new-session -d -s "$S" -x 160 -y 48 "script -q -f -c 'ACCEL=kvm sh $D/run-console.sh $D/vmlinuz $D/initramfs.cpio.gz $D/btrfs.img' $LOG; sleep 1"
	waitfor "$LOG" 'SPACEWAVE_OS_CONSOLE_READY' 240
	waitfor "$LOG" 'spacewave# ' 30
	note "boot $1 ready qemu=$(qpid) $(tr -d '\r' < "$LOG" | grep -E 'SPACEWAVE_OS_REOPEN' | tail -1)"
	send "stty -echo; echo ECHO_\$((1+1))_OFF"
	waitfor "$LOG" 'ECHO_2_OFF' 30
}
killmid() { # marker regex
	waitfor "$LOG" "$1" 600
	local pid w0 w
	pid=$(qpid)
	w0=$(awk '/^write_bytes/{print $2}' /proc/$pid/io)
	while :; do
		w=$(awk '/^write_bytes/{print $2}' /proc/$pid/io)
		[ "$w" -gt "$w0" ] && break
		sleep 0.01
	done
	kill -9 "$pid"
	note "SIGKILL qemu pid=$pid write_bytes start=$w0 seen=$w"
	while kill -0 "$pid" 2>/dev/null; do sleep 0.05; done
	tmux kill-session -t "$S" 2>/dev/null || true
}
acks() { tr -d '\r' < "$1" | grep -o -E 'ACK 262144 [0-9a-f]{64}  /data/[a-z-]+/record-[0-9]+' | awk '{print $3"  "$4}'; }
writer() { # dir marker-word
	cat <<SCRIPT
cat > /tmp/w.sh <<'EOF'
set -eu
mkdir /data/$1
sync /data
for i in \$(seq 1 16); do
  path=/data/$1/record-\$i
  dd if=/dev/urandom of=/tmp/payload bs=262144 count=1 2>/dev/null
  hash=\$(sha256sum /tmp/payload)
  hash=\${hash%% *}
  cat /tmp/payload > "\$path"
  sync "\$path"
  sync /data/$1
  echo "ACK 262144 \$hash  \$path"
done
dd if=/dev/urandom of=/data/$1/inflight bs=1048576 count=2 2>/dev/null
printf '\nFINAL_CRASH_%s_START\n' $2
exec dd if=/dev/urandom of=/data/$1/inflight bs=1048576 count=256 seek=2 conv=notrunc
EOF
sh /tmp/w.sh
SCRIPT
}
checker() { # ackfile count dirs...
	# Each test stops the subshell with a failure, so only a full match prints
	# ACK_OK. ash ignores set -e inside a list such as "( ... ) || echo".
	local f=$1 n=$2 globs="" d; shift 2
	for d in "$@"; do globs="$globs /data/$d/record-*"; done
	echo "cat > /tmp/acknowledged <<'ACKS'"
	cat "$f"
	echo "ACKS"
	echo "if (sha256sum -c /tmp/acknowledged >/dev/null || exit 1; for p in$globs; do test \"\$(stat -c %s \"\$p\")\" = 262144 || exit 1; done; test \$(ls$globs | wc -l) = $n); then echo ACK_OK_\$((0+$n)); else echo CHECK_FAILED_\$((1+1)); fi"
	for d in "$@"; do echo "stat -c 'PARTIAL_BYTES %n %s' /data/$d/inflight 2>/dev/null || echo PARTIAL_BYTES /data/$d/inflight absent"; done
}

note "trial start dir=$D host=$(hostname) qemu=$(qemu-system-x86_64 --version | head -1)"
sha256sum vmlinuz initramfs.cpio.gz run-console.sh | tee -a "$H"

# Trial 1: 16 acknowledged records, then SIGKILL during a further write.
boot boot1-crash1.log
send "$(writer records ONE)"
killmid 'FINAL_CRASH_ONE_START'
acks boot1-crash1.log > acks1.txt
note "trial 1 acknowledged $(wc -l < acks1.txt)"
[ "$(wc -l < acks1.txt)" = 16 ]

# Reopen: check trial 1, then trial 2 with hashes taken before the persistent write.
boot boot2-check1-crash2.log
send "$(checker acks1.txt 16 records)"
waitfor "$LOG" 'ACK_OK_16|CHECK_FAILED_2' 120
tr -d '\r' < "$LOG" | grep -q ACK_OK_16 || { note "trial 1 check FAILED"; exit 1; }
note "trial 1 check ok: $(tr -d '\r' < "$LOG" | grep PARTIAL_BYTES | tail -1)"
send "$(writer input-records TWO)"
killmid 'FINAL_CRASH_TWO_START'
acks boot2-check1-crash2.log > acks2.txt
note "trial 2 acknowledged $(wc -l < acks2.txt)"
[ "$(wc -l < acks2.txt)" = 16 ]
cat acks1.txt acks2.txt > acks-all.txt

# Reopen: check all 32, then a clean poweroff.
boot boot3-check-all.log
send "$(checker acks-all.txt 32 records input-records)"
waitfor "$LOG" 'ACK_OK_32|CHECK_FAILED_2' 120
tr -d '\r' < "$LOG" | grep -q ACK_OK_32 || { note "all-records check FAILED"; exit 1; }
note "all-records check ok: $(tr -d '\r' < "$LOG" | grep PARTIAL_BYTES | tail -2 | tr '\n' ' ')"
send "sync; exit"
waitfor "$LOG" 'reboot: Power down' 120
note "clean poweroff: $(tr -d '\r' < "$LOG" | grep -E 'SPACEWAVE_OS_(PASS|STORAGE_CLOSED)' | tr '\n' ' ')"
tr -d '\r' < boot1-crash1.log boot2-check1-crash2.log boot3-check-all.log 2>/dev/null | grep -E 'SPACEWAVE_OS_REOPEN' | sort | uniq -c | tee -a "$H"
note "KILL_TRIAL_PASS"

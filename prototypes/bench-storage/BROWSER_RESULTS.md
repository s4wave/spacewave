2026/03/31 20:03:05 ERROR: could not unmarshal event: unknown IPAddressSpace value: Loopback
2026/03/31 20:03:05 ERROR: could not unmarshal event: unknown IPAddressSpace value: Loopback
2026/03/31 20:03:05 ERROR: could not unmarshal event: unknown IPAddressSpace value: Loopback
2026/03/31 20:03:05 ERROR: could not unmarshal event: unknown IPAddressSpace value: Loopback
2026/03/31 20:03:05 ERROR: could not unmarshal event: unknown IPAddressSpace value: Loopback
2026/03/31 20:03:05 ERROR: could not unmarshal event: unknown IPAddressSpace value: Loopback
2026/03/31 20:03:05 ERROR: could not unmarshal event: unknown IPAddressSpace value: Loopback
goos: js
goarch: wasm
pkg: github.com/s4wave/spacewave/prototypes/bench-storage
BenchmarkBlake3/4KiB              	  181816	      6584 ns/op	 622.15 MB/s	       0 B/op	       0 allocs/op
BenchmarkBlake3/64KiB             	   12016	    100383 ns/op	 652.86 MB/s	       0 B/op	       0 allocs/op
BenchmarkBlake3/1MiB              	     739	   1632070 ns/op	 642.48 MB/s	       0 B/op	       0 allocs/op
BenchmarkSHA256/4KiB              	   65252	     18294 ns/op	 223.90 MB/s	       0 B/op	       0 allocs/op
BenchmarkSHA256/64KiB             	    4166	    285142 ns/op	 229.84 MB/s	       0 B/op	       0 allocs/op
BenchmarkSHA256/1MiB              	     260	   4545000 ns/op	 230.71 MB/s	       0 B/op	       0 allocs/op
BenchmarkIndexedDBKVWrite/4KiB    	    2419	    430591 ns/op	   9.51 MB/s	   18672 B/op	     418 allocs/op
BenchmarkIndexedDBKVWrite/64KiB   	    3076	    505267 ns/op	 129.71 MB/s	   80112 B/op	     418 allocs/op
BenchmarkIndexedDBKVWriteSingleTx/4KiB         	    4123	    341620 ns/op	  11.99 MB/s	   11544 B/op	     333 allocs/op
BenchmarkIndexedDBKVWriteSingleTx/64KiB        	    3518	    351961 ns/op	 186.20 MB/s	   11544 B/op	     333 allocs/op
BenchmarkIndexedDBKVReadTxPerOp/4KiB           	   10000	    121110 ns/op	  33.82 MB/s	    8096 B/op	     116 allocs/op
BenchmarkIndexedDBKVReadTxPerOp/64KiB          	    6975	    177577 ns/op	 369.06 MB/s	   69536 B/op	     116 allocs/op
BenchmarkIndexedDBKVReadSingleTx/4KiB          	   13678	     89816 ns/op	  45.60 MB/s	    6928 B/op	      82 allocs/op
BenchmarkIndexedDBKVReadSingleTx/64KiB         	    9301	    135899 ns/op	 482.24 MB/s	   68368 B/op	      82 allocs/op
BenchmarkIndexedDBUnixFSWriteFile/4KiB         	       3	 339700053 ns/op	   0.01 MB/s	19213450 B/op	  489240 allocs/op
BenchmarkIndexedDBUnixFSWriteFile/64KiB        	       3	 358400000 ns/op	   0.18 MB/s	21035386 B/op	  516396 allocs/op
PASS
ok  	github.com/s4wave/spacewave/prototypes/bench-storage	47.801s

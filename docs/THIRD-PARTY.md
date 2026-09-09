# Third-Party Tool Binaries

vmbench itself is MIT-licensed. The static benchmark tool binaries that
`vmbench tools fetch` optionally provisions are separate upstream programs
redistributed under their own licenses. They are invoked as independent
executables; vmbench neither links against nor derives from their code.

| Tool | Version | Upstream license | Source |
|---|---|---|---|
| fio | 3.39 | GPLv2 | https://github.com/axboe/fio/tree/fio-3.39 |
| sysbench | 1.0.20 | GPLv2 | https://github.com/akopytov/sysbench/tree/1.0.20 |

## Source offer

Both tools are GPL-2.0. The exact corresponding source for every binary
published under the stable `tools` release tag is the upstream tag listed
above, built unmodified by [`scripts/build-tools.sh`](../scripts/build-tools.sh)
(Alpine musl static builds; sysbench additionally pulls its own pinned git
submodules — LuaJIT and ConcurrencyKit — at that tag). Anyone who received a
binary can reproduce it bit-for-bit from those inputs, or obtain the complete
corresponding source from the upstream repositories at no charge.

## Integrity

Each `vmbench-tools-<tool>-linux-<arch>` asset is verified at download time
against a SHA-256 pin compiled into `toolbin/toolbin.go`; a mismatch fails
closed and nothing is installed. Repinning follows the normal review process:
rebuild the assets, upload them with `gh release upload tools --clobber`,
and update the hashes in the same change.

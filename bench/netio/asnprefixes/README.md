# China carrier ASN prefix snapshots

IPv6 prefix lists for the China carrier backbone ASNs used by the hop ASN
annotation in `bench/netio/hopasn.go`.

- Source: [oneclickvirt/backtrace](https://github.com/oneclickvirt/backtrace)
  `bk/prefix/as*.txt` (Apache-2.0), originally generated from public BGP
  looking-glass data. Copied unmodified as a versioned data snapshot.
- License: Apache-2.0 (data files). The matching code in vmbench is an
  independent implementation (`net/netip` longest-match).
- Refresh: copy the current `bk/prefix/as*.txt` files over these and bump the
  comment on `asnPrefixFS` in `hopasn.go` with the backtrace version used.

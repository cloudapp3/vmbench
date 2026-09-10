package nodecatalog

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DefaultManifestURLs is the whitespace-separated ordered mirror chain used
// by the auto source: the canonical raw URL first, then CN-reachable mirrors
// (the ecsspeed distribution model). It is a plain string literal (not
// strings.Join) so -ldflags -X can replace it wholesale — the linker only
// overrides constant-initialized vars. Documents are fetched over HTTPS and
// must pass the strict schema decoder; mirrors are plain distribution
// endpoints and can be added or removed freely.
var DefaultManifestURLs = "https://raw.githubusercontent.com/cloudapp3/vmbench/main/nodecatalog/nodes.json https://cdn.jsdelivr.net/gh/cloudapp3/vmbench@main/nodecatalog/nodes.json https://gh-proxy.com/https://raw.githubusercontent.com/cloudapp3/vmbench/main/nodecatalog/nodes.json https://cdn.spiritlhl.net/https://raw.githubusercontent.com/cloudapp3/vmbench/main/nodecatalog/nodes.json"

var (
	// DefaultFetchTimeout bounds the whole mirror walk, not one mirror.
	DefaultFetchTimeout = 5 * time.Second
	// DefaultFetchAttemptTimeout caps each non-final mirror so a hanging
	// early mirror (raw.githubusercontent.com is commonly unreachable from
	// CN) cannot eat the shared budget; the final mirror gets the remainder.
	DefaultFetchAttemptTimeout = 2 * time.Second
)

func manifestURLs() []string {
	return strings.Fields(DefaultManifestURLs)
}

func remoteConfigured() bool {
	return len(manifestURLs()) > 0
}

// attemptContext derives one mirror's deadline from the shared budget: the
// soft per-mirror cap unless little remains or this is the final mirror,
// which gets whatever is left.
func attemptContext(parent context.Context, index, total int) (context.Context, context.CancelFunc) {
	remaining := DefaultFetchTimeout
	if deadline, ok := parent.Deadline(); ok {
		remaining = time.Until(deadline)
	}
	if index == total-1 || remaining <= DefaultFetchAttemptTimeout {
		return context.WithTimeout(parent, remaining)
	}
	return context.WithTimeout(parent, DefaultFetchAttemptTimeout)
}

// loadRemote walks the mirror chain under one shared timeout and returns the
// first manifest that passes bounded download + strict schema + revision
// pin; on success it atomically caches the exact bytes (a best-effort
// offline fallback — a write failure only warns, the fetched manifest still
// wins). Every failure mode (network, schema-invalid, pin mismatch) returns
// an error whose only purpose is the caller's silent fallback; earlier
// mirror failures are forgotten once a later mirror succeeds. Concurrent
// loadRemote calls are safe: atomicWrite is temp+rename, so readers never
// see partial files and the last writer wins.
func loadRemote(cachePath, pin string) (Loaded, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultFetchTimeout)
	defer cancel()

	urls := manifestURLs()
	var lastErr error
	for i, manifestURL := range urls {
		attemptCtx, attemptCancel := attemptContext(ctx, i, len(urls))
		data, err := fetch(attemptCtx, http.DefaultClient, manifestURL, maxManifestBytes)
		attemptCancel()
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", manifestURL, err)
		} else if manifest, decodeErr := Decode(data); decodeErr != nil {
			lastErr = fmt.Errorf("%s: %w", manifestURL, decodeErr)
		} else if revisionErr := checkRevision(manifest, pin); revisionErr != nil {
			// A pin asks for exactly one revision by any means; keep the
			// pinned cache intact instead of clobbering it with the new one.
			lastErr = fmt.Errorf("%s: %w", manifestURL, revisionErr)
		} else {
			loaded := Loaded{
				Manifest: manifest,
				Source:   SourceRemote,
				Warning:  expirationWarning(manifest, time.Now()),
				Raw:      append([]byte(nil), data...),
			}
			if writeErr := atomicWrite(cachePath, data, 0o600); writeErr != nil {
				loaded.Warning = joinWarnings(loaded.Warning, fmt.Sprintf("cached catalog not updated: %v", writeErr))
			}
			return loaded, nil
		}
		if ctx.Err() != nil {
			break // shared budget spent
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no mirrors configured")
	}
	return Loaded{}, lastErr
}

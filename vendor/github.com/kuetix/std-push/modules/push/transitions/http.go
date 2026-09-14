package transitions

import (
	"net/http"
	"sync"
	"time"
)

var (
	httpOnce   sync.Once
	httpShared *http.Client
)

// httpClient is shared by the Web Push and APNs senders. Transport is left
// nil so it uses http.DefaultTransport, which has ForceAttemptHTTP2 = true
// - APNs negotiates HTTP/2 over ALPN with api.push.apple.com on its own,
// no explicit http2.Transport (and no golang.org/x/net dependency) needed.
func httpClient() *http.Client {
	httpOnce.Do(func() {
		httpShared = &http.Client{Timeout: 10 * time.Second}
	})
	return httpShared
}

func apnsClient() *http.Client { return httpClient() }

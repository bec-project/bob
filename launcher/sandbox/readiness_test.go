package sandbox

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"
)

// Use in-memory connections so proxy/redirect regressions need no live sandbox
// or network listener. A proxy response must never substitute for sandbox health.
func TestReadinessUsesDirectConnectionAndDoesNotFollowRedirects(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		t.Run(fmt.Sprint("redirect=", redirect), func(t *testing.T) {
			original := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = original })
			var mu sync.Mutex
			var addresses []string
			var servers sync.WaitGroup
			proxy, _ := url.Parse("http://proxy.invalid:8080")
			http.DefaultTransport = &http.Transport{
				Proxy: func(*http.Request) (*url.URL, error) { return proxy, nil },
				DialContext: func(_ context.Context, _, address string) (net.Conn, error) {
					mu.Lock()
					addresses = append(addresses, address)
					mu.Unlock()
					client, server := net.Pipe()
					servers.Add(1)
					go func() {
						defer servers.Done()
						defer server.Close()
						server.SetDeadline(time.Now().Add(time.Second))
						request, err := http.ReadRequest(bufio.NewReader(server))
						if err != nil {
							return
						}
						request.Body.Close()
						if redirect && address == "192.0.2.1:12345" {
							fmt.Fprint(server, "HTTP/1.1 302 Found\r\nLocation: http://other.invalid:8080/global/health\r\nContent-Length: 0\r\n\r\n")
						} else {
							fmt.Fprint(server, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
						}
					}()
					return client, nil
				},
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			err := waitReady(ctx, Connection{URL: "http://192.0.2.1:12345", Password: "synthetic", Runtime: "sh", Name: "synthetic"})
			servers.Wait()
			if redirect && err == nil {
				t.Fatal("redirect target substituted for sandbox health")
			}
			if !redirect && err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(addresses) == 0 {
				t.Fatal("sandbox health was never checked")
			}
			for _, address := range addresses {
				if address != "192.0.2.1:12345" {
					t.Fatalf("credentials could reach proxy or redirect target: %s", address)
				}
			}
		})
	}
}

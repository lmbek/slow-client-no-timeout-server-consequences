package main

// can be run with:
// go run fast_client.go -reqs=200 -parallel=16 -size=20

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

func main() {
	var (
		url      = flag.String("url", "http://localhost:8080/upload", "target URL")
		size     = flag.Int("size", 1<<20, "payload size in bytes (default 1 MiB)")
		conns    = flag.Int("conns", 32, "max idle connections / per-host")
		reqs     = flag.Int("reqs", 100000, "total number of requests to send")
		parallel = flag.Int("parallel", 8, "number of concurrent workers")
		timeout  = flag.Duration("timeout", 30*time.Second, "per-request timeout")
	)
	flag.Parse()

	// Optional port override via query parameter: ?port=NNNN
	if newURL, err := applyPortOverride(*url); err == nil {
		*url = newURL
	} else {
		log.Fatalf("invalid url: %v", err)
	}

	payload := bytes.Repeat([]byte{'A'}, *size)

	// Dialer tuned for quick connect + keep-alive
	dialer := &net.Dialer{
		Timeout:   3 * time.Second,
		KeepAlive: 60 * time.Second,
	}

	// Transport tuned for throughput and connection reuse
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     false, // your server is likely HTTP/1.1 on :8080
		MaxIdleConns:          *conns,
		MaxIdleConnsPerHost:   *conns,
		MaxConnsPerHost:       0, // unlimited; rely on backpressure
		IdleConnTimeout:       90 * time.Second,
		DisableCompression:    true,            // avoid gzip overhead unless you need it
		ExpectContinueTimeout: 0 * time.Second, // disable 100-continue wait
		WriteBufferSize:       1 << 20,         // 1 MiB buffers help large bodies
		ReadBufferSize:        1 << 20,
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   *timeout,
	}

	start := time.Now()
	var ok, fail int64
	wg := sync.WaitGroup{}
	jobs := make(chan int, *reqs)

	// workers
	for w := 0; w < *parallel; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				// Use bytes.Reader so Go sets Content-Length (=> no chunked encoding).
				br := bytes.NewReader(payload)
				ctx, cancel := context.WithTimeout(context.Background(), *timeout)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, *url, br)
				if err != nil {
					cancel()
					log.Printf("new request: %v", err)
					fail++
					continue
				}
				req.Header.Set("Content-Type", "application/octet-stream")
				req.ContentLength = int64(br.Len()) // explicit, though http sets it for bytes.Reader

				resp, err := client.Do(req)
				if err != nil {
					cancel()
					log.Printf("request error: %v", err)
					fail++
					continue
				}
				// Drain + discard to allow connection reuse.
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				cancel()

				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					ok++
				} else {
					fail++
				}
			}
		}()
	}

	// enqueue jobs
	for i := 0; i < *reqs; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	elapsed := time.Since(start)

	fmt.Printf("done: ok=%d fail=%d in %v (%.2f req/s)\n",
		ok, fail, elapsed, float64(*reqs)/elapsed.Seconds())
}

// applyPortOverride allows specifying ?port=NNNN in the URL to override its network port.
// It removes the port query parameter from the final request URL.
func applyPortOverride(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	p := q.Get("port")
	if p == "" {
		return raw, nil
	}
	q.Del("port")
	u.RawQuery = q.Encode()
	// Use hostname (without any existing port) and join with the override port
	hostname := u.Hostname()
	if hostname == "" {
		hostname = u.Host
	}
	u.Host = net.JoinHostPort(hostname, p)
	return u.String(), nil
}

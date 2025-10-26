// client_slow/main.go
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

func main() {
	var (
		url  = flag.String("url", "http://localhost:8080/upload", "target URL")
		size = flag.Int("size", 10*1024, "payload size in bytes (total to send)")
		//duration = flag.Duration("duration", 10*time.Millisecond, "how long to take sending each request body")
		duration = flag.Duration("duration", 100*time.Second, "how long to take sending each request body")

		chunks   = flag.Int("chunks", 20, "number of chunks to split the body into")
		reqs     = flag.Int("reqs", 100000000, "total number of requests to send")
		parallel = flag.Int("parallel", 200, "number of concurrent workers")
		timeout  = flag.Duration("timeout", 1*time.Millisecond, "per-request timeout (should exceed duration)")
		conns    = flag.Int("conns", 150, "max idle connections / per-host")
		forceH1  = flag.Bool("h1", true, "force HTTP/1.1 (set false to allow HTTP/2 over TLS)")
	)
	flag.Parse()

	if *chunks <= 0 {
		log.Fatal("chunks must be > 0")
	}
	if *size <= 0 {
		log.Fatal("size must be > 0")
	}
	if *timeout <= *duration {
		log.Printf("Note: timeout (%v) <= duration (%v). Adding headroom.", *timeout, *duration)
		*timeout = *duration + 10*time.Second
	}

	// Pre-make one chunk pattern to copy from (no CPU in the write loop)
	fill := bytes.Repeat([]byte("A"), min(1<<16, *size)) // up to 64 KiB source buffer

	dialer := &net.Dialer{
		Timeout:   3 * time.Second,
		KeepAlive: 60 * time.Second,
	}
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         dialer.DialContext,
		ForceAttemptHTTP2:   !*forceH1, // default true for TLS; we keep false by default for localhost:8080
		MaxIdleConns:        *conns,
		MaxIdleConnsPerHost: *conns,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		// Leave ExpectContinueTimeout at default; we don't send Expect header anyway.
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   *timeout,
	}

	start := time.Now()
	var ok, fail int64
	wg := sync.WaitGroup{}
	jobs := make(chan int, *reqs)

	// Workers
	for w := 0; w < *parallel; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				if err := doSlowRequest(client, *url, *size, *duration, *chunks, fill, *timeout); err != nil {
					log.Printf("request error: %v", err)
					fail++
				} else {
					ok++
				}
			}
		}()
	}

	// Enqueue jobs
	for i := 0; i < *reqs; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	elapsed := time.Since(start)
	fmt.Printf("done: ok=%d fail=%d in %v (%.2f req/s)\n",
		ok, fail, elapsed, float64(*reqs)/elapsed.Seconds())
}

func doSlowRequest(client *http.Client, url string, total int, dur time.Duration, chunks int, fill []byte, timeout time.Duration) error {
	pr, pw := io.Pipe()

	// Writer goroutine: drip exactly 'total' bytes over 'dur' in 'chunks' steps.
	go func() {
		defer pw.Close()

		// pacing
		perChunkDur := dur / time.Duration(chunks)
		timer := time.NewTimer(0)
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		remaining := total
		perChunk := total / chunks
		remainder := total % chunks

		for i := 0; i < chunks; i++ {
			toWrite := perChunk
			if i == chunks-1 {
				toWrite += remainder
			}
			if err := writeRepeated(pw, fill, toWrite); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			remaining -= toWrite
			if i < chunks-1 {
				timer.Reset(perChunkDur)
				<-timer.C
			}
		}
		_ = remaining // should be zero
	}()

	// Build request; we *know* the total size, so set Content-Length to avoid chunked TE.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(total)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drain for connection reuse

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	return nil
}

// writeRepeated writes exactly n bytes to w by repeating src (without allocations).
func writeRepeated(w io.Writer, src []byte, n int) error {
	for n > 0 {
		chunk := len(src)
		if chunk > n {
			chunk = n
		}
		if _, err := w.Write(src[:chunk]); err != nil {
			return err
		}
		n -= chunk
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// client_semifast_forever/main.go
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
	"net/url"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

// writeRepeated writes exactly n bytes to w by repeating src.
func writeRepeated(w io.Writer, src []byte, n int64) error {
	for n > 0 {
		chunk := int64(len(src))
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

func doSemifastRequest(ctx context.Context, client *http.Client, url string, totalBytes int64, rateBytesPerSec int64, duration time.Duration, tick time.Duration, timeout time.Duration, fill []byte) error {
	pr, pw := io.Pipe()

	// Writer: smooth the sending across ticks so each 1s interval completes within the second.
	go func() {
		defer pw.Close()

		remaining := totalBytes
		if tick <= 0 {
			tick = 100 * time.Millisecond
		}
		totalTicks := int(duration / tick)
		if totalTicks <= 0 {
			totalTicks = 1
		}

		// bytes per tick (base); we'll send remainder on later ticks so total matches exactly.
		perTickFloat := float64(rateBytesPerSec) * float64(tick) / float64(time.Second)
		perTickBase := int64(perTickFloat)

		t := time.NewTicker(tick)
		defer t.Stop()

		ticksDone := 0
		for ticksDone < totalTicks && remaining > 0 {
			// compute bytes to send this tick
			toSend := perTickBase
			// On the last tick send everything remaining for this request (avoids rounding issues).
			if ticksDone == totalTicks-1 {
				toSend = remaining
			} else if toSend > remaining {
				toSend = remaining
			}

			if toSend > 0 {
				if err := writeRepeated(pw, fill, toSend); err != nil {
					_ = pw.CloseWithError(err)
					return
				}
				remaining -= toSend
			}

			ticksDone++
			// wait for next tick or ctx cancellation
			if ticksDone < totalTicks && remaining > 0 {
				select {
				case <-t.C:
				case <-ctx.Done():
					_ = pw.CloseWithError(ctx.Err())
					return
				}
			}
		}
		// final remainder if any
		if remaining > 0 {
			_ = writeRepeated(pw, fill, remaining)
			remaining = 0
		}
	}()

	// Build request with known Content-Length (no chunked transfer)
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, pr)
	if err != nil {
		_ = pr.Close()
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = totalBytes

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}
	return nil
}

func main() {
	var (
		url      = flag.String("url", "http://localhost:8080/upload", "target URL")
		rateMB   = flag.Int("rateMB", 1, "rate in MiB per second per request")
		duration = flag.Duration("duration", 1*time.Second, "how long each request should stream for")
		parallel = flag.Int("parallel", 4, "number of concurrent workers (each worker repeats forever)")
		timeout  = flag.Duration("timeout", 30*time.Second, "per-request timeout (should exceed duration)")
		conns    = flag.Int("conns", 100, "max idle connections / per-host")
		tickMs   = flag.Int("tickMs", 100, "tick interval in milliseconds for smoothing")
		fillSz   = flag.Int("fillSz", 32*1024, "internal fill buffer size in bytes (sub-write size)")
		forceH1  = flag.Bool("h1", true, "force HTTP/1.1 (false allows HTTP/2 over TLS)")
		gapMs    = flag.Int("gapMs", 0, "millisecond gap between requests by the same worker (0 = start next immediately)")
	)
	flag.Parse()

	// Optional port override via query parameter: ?port=NNNN
	if newURL, err := applyPortOverride(*url); err == nil {
		*url = newURL
	} else {
		log.Fatalf("invalid url: %v", err)
	}

	if *rateMB <= 0 || *duration <= 0 || *parallel <= 0 {
		logFatal("invalid flags: rateMB, duration and parallel must be > 0")
	}
	if *timeout <= *duration {
		*timeout = *duration + 10*time.Second
	}

	rateBytes := int64(*rateMB) * 1024 * 1024
	totalBytes := rateBytes * int64((*duration).Seconds())
	fill := bytes.Repeat([]byte("A"), *fillSz)
	tick := time.Duration(*tickMs) * time.Millisecond
	gap := time.Duration(*gapMs) * time.Millisecond

	// Transport tuned for concurrency
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 60 * time.Second}
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     !*forceH1,
		MaxIdleConns:          *conns,
		MaxIdleConnsPerHost:   *conns,
		IdleConnTimeout:       90 * time.Second,
		DisableCompression:    true,
		ExpectContinueTimeout: 0,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   *timeout,
	}

	// graceful shutdown on SIGINT/SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var successCount int64
	var failCount int64

	log.Printf("starting %d workers, each sending %d MiB/s for %v per request (total bytes per request: %d). Ctrl+C to stop.",
		*parallel, *rateMB, *duration, totalBytes)

	// spawn workers
	for w := 0; w < *parallel; w++ {
		go func(id int) {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// run one request
					err := doSemifastRequest(ctx, client, *url, totalBytes, rateBytes, *duration, tick, *timeout, fill)
					if err != nil {
						atomic.AddInt64(&failCount, 1)
						log.Printf("[w%d] request error: %v", id, err)
					} else {
						atomic.AddInt64(&successCount, 1)
					}
					// optional gap between requests from same worker
					if gap > 0 {
						select {
						case <-time.After(gap):
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}(w)
	}

	// simple status printer until canceled
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("shutdown signal received, waiting briefly for inflight requests to finish...")
			// give a short grace period for inflight requests (they will be canceled by ctx)
			time.Sleep(500 * time.Millisecond)
			log.Printf("final: success=%d fail=%d\n", atomic.LoadInt64(&successCount), atomic.LoadInt64(&failCount))
			return
		case <-ticker.C:
			log.Printf("running: success=%d fail=%d\n", atomic.LoadInt64(&successCount), atomic.LoadInt64(&failCount))
		}
	}
}

func logFatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(2)
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
	hostname := u.Hostname()
	if hostname == "" {
		hostname = u.Host
	}
	u.Host = net.JoinHostPort(hostname, p)
	return u.String(), nil
}

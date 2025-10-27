package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"
)

func main() {
	var target = flag.String("url", "http://localhost:8080/upload", "target URL")
	flag.Parse()
	// Optional port override via query parameter: ?port=NNNN
	if newURL, err := applyPortOverride(*target); err == nil {
		*target = newURL
	} else {
		log.Fatalf("invalid url: %v", err)
	}

	pr, pw := io.Pipe()

	// Writer goroutine: drip data across ~10s
	go func() {
		defer pw.Close()

		totalDuration := 100 * time.Second
		chunks := 20
		perChunk := totalDuration / time.Duration(chunks)

		chunk := bytes.Repeat([]byte("A"), 102400) // 1 KiB per chunk => ~20 KiB total
		for i := 0; i < chunks; i++ {
			// Write one chunk, then wait a bit
			if _, err := pw.Write(chunk); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			time.Sleep(perChunk)
		}
	}()

	req, err := http.NewRequest("POST", *target, pr)
	if err != nil {
		log.Fatal(err)
	}
	// ContentLength left at -1 (unknown) => Go will use chunked transfer-encoding.

	client := &http.Client{
		Timeout: 45 * time.Second, // > server timeout
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("server responded: %s\n", string(body))
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

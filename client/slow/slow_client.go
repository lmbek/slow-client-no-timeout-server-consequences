package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func main() {
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

	req, err := http.NewRequest("POST", "http://localhost:8080/upload", pr)
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

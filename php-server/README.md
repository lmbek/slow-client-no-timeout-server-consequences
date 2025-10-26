PHP server (Apache)

Overview
- Minimal PHP app served by Apache (no/very high timeouts for testing only).
- Endpoints:
  - GET /           -> "hej med dig"
  - POST /upload    -> streams request body (no buffering), counts bytes, replies with timing

Run it (Docker)
1) From repo root (recommended):
   - make up        # builds and starts via docker compose on localhost:8080
   - make down      # stop
   Or manual build/run:
   - cd php-server
   - docker build -t slow-php .
   - docker run --rm -p 8080:8080 slow-php

Quick test
- Open http://localhost:8080/ (expect: hej med dig)
- Upload test (Windows PowerShell):
  PS> $b = New-Object byte[] (1MB); [void](New-Object Random).NextBytes($b)
  PS> Invoke-WebRequest -Uri http://localhost:8080/upload -Method POST -Body $b

Warning
- This configuration intentionally removes common protections/timeouts. Do NOT use in production.

<?php
// Simple PHP endpoint with effectively no timeouts, to be hammered by slow/fast clients.
// Endpoint: POST /upload
// Behavior: streams request body from php://input without buffering, counts bytes, replies with timing stats.
// WARNING: This intentionally disables common protections/timeouts. Do NOT use in production.

// Remove time limits in PHP layer
@ini_set('max_execution_time', '0');       // unlimited
@ini_set('max_input_time', '-1');          // unlimited
@ini_set('default_socket_timeout', '-1');  // unlimited
@ini_set('output_buffering', '0');
@ini_set('implicit_flush', '1');
@ini_set('zlib.output_compression', '0');

ignore_user_abort(true); // continue even if the client disconnects
set_time_limit(0);        // another way to enforce no time limit

// Disable PHP output buffering at runtime, if any
while (ob_get_level() > 0) {
    ob_end_flush();
}
ob_implicit_flush(true);

$uri = parse_url($_SERVER['REQUEST_URI'] ?? '/', PHP_URL_PATH);
$method = $_SERVER['REQUEST_METHOD'] ?? 'GET';

if ($uri !== '/upload' || $method !== 'POST') {
    http_response_code(404);
    header('Content-Type: text/plain');
    echo "not found\n";
    exit;
}

$start = microtime(true);
$bytes = 0;

$in = fopen('php://input', 'rb');
if ($in === false) {
    http_response_code(400);
    echo "could not open input stream\n";
    exit;
}

// Stream read in chunks to avoid memory growth; tolerate very slow senders.
$bufSize = 8192; // 8 KiB
while (!feof($in)) {
    $chunk = fread($in, $bufSize);
    if ($chunk === false) {
        http_response_code(400);
        echo "read error\n";
        fclose($in);
        exit;
    }
    if ($chunk === '') {
        // Avoid busy loop if the client is pausing
        usleep(10_000); // 10 ms
        continue;
    }
    $bytes += strlen($chunk);
}

fclose($in);
$duration = microtime(true) - $start;

header('Content-Type: text/plain');
http_response_code(200);
printf("ok: read %d bytes in %.3f seconds\n", $bytes, $duration);

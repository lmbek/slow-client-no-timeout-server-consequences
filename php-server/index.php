<?php
// Measure request body read time and bytes, similar to the Go server
// Responds with: ok: read <n> bytes in <duration>
// Logs the same line to the PHP error log.

// Always produce plain text output
header('Content-Type: text/plain');

$start = microtime(true);

$stream = fopen('php://input', 'rb');
if ($stream === false) {
    http_response_code(400);
    echo "read error: cannot open request body\n";
    error_log('read error: cannot open request body');
    exit;
}

$total = 0;
$err = null;
while (!feof($stream)) {
    $chunk = fread($stream, 8192);
    if ($chunk === false) {
        $err = 'read error: failed to read from request body';
        break;
    }
    if ($chunk === '') {
        // Avoid tight loop in rare cases
        usleep(1000);
        continue;
    }
    $total += strlen($chunk);
}

fclose($stream);

$durSeconds = microtime(true) - $start;
// Format like Go's time.Duration string approximately: seconds with 6 decimals and trailing 's'
$durStr = number_format($durSeconds, 6, '.', '') . 's';

if ($err !== null) {
    http_response_code(400);
    echo $err . "\n";
    error_log($err);
    exit;
}

$msg = sprintf("ok: read %d bytes in %s\n", $total, $durStr);
error_log($msg);
echo $msg;
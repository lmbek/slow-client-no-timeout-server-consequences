<?php
// Router script for PHP built-in server.
// - Serves existing static files directly (by returning false)
// - Otherwise dispatches everything to index.php
// This lets us handle routes like /upload when running `php -S ... router.php`.

if (php_sapi_name() === 'cli-server') {
    $path = parse_url($_SERVER['REQUEST_URI'] ?? '/', PHP_URL_PATH);
    $file = __DIR__ . $path;
    if ($path !== '/' && is_file($file)) {
        // Let the built-in server handle the static file request
        return false;
    }
}

require __DIR__ . '/index.php';

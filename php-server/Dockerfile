# Apache-based PHP server for demo. No/very high timeouts (testing only!)
# WARNING: Do NOT expose to the internet.

FROM php:8.2-apache

# Put app in /var/www/html
WORKDIR /var/www/html
COPY index.php /var/www/html/index.php
# Restore /upload endpoint under Apache
COPY upload/ /var/www/html/upload/

# Apache on 8080 and very high/no timeouts
COPY ports.conf /etc/apache2/ports.conf
COPY 000-default.conf /etc/apache2/sites-available/000-default.conf

# Disable mod_reqtimeout (request read/write timeouts) to avoid timeouts under slow clients
RUN a2dismod -f reqtimeout || true

# PHP runtime: disable/raise timeouts
COPY no-timeouts.ini /usr/local/etc/php/conf.d/no-timeouts.ini

# Expose the HTTP port
EXPOSE 8080

# Use the default apache2-foreground entrypoint from the base image

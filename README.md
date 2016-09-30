# phpfpmtop

phpfpmtop is a top-inspired processviewer for PHP-FPM. 

![Screenshot](https://cloud.githubusercontent.com/assets/3726205/18997238/c71415ee-8733-11e6-8566-3bd4d96ca775.png)

## Installation

If you're not using a prebuilt version of phpfpmtop, you will need a working
Go environment to build and install.

If your GOPATH is properly configured, you can install and compile phpfpmtop like this:

    go get github.com/abrander/phpfpmtop

## Configuration

Configuration is a single TOML/INI style file located at `~/.phpfpmtop.conf`.
If you start phpfpmtop without a configuration file, one will be created for
you with sensible defaults.

| Key    | Default value                 | Description                                                                                       |
|--------|-------------------------------|---------------------------------------------------------------------------------------------------|
| listen | "/var/run/php5-fpm.sock"      | The path to FPM. Can be in the form of a UNIX socket or a TCP address in the form 'address:port'. |
| status | "/status"                     | The URI for the PHP-FPM status page as defined in php-fpm configuration.                          |
| url    | none                          | If the PHP-FPM status page is available via http/https/http2 this URL will be used.               |

Have fun :-)

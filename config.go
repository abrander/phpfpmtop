package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"syscall"

	"github.com/BurntSushi/toml"
)

type (
	config struct {
		StatusPath string `toml:"status"`
		ListenPath string `toml:"listen"`
	}
)

var (
	configs map[string]config
)

const (
	// configFilename is the name of the configuration file loaded from the
	// user's home directory.
	configFilename = ".phpfpmtop.conf"

	// defaultConfig will be written to configFilename if none is found.
	defaultConfig = `# Default section will be used if there's no arguments given to phpfpmtop.
[default]
listen = "/var/run/php5-fpm.sock"    # The value of the "listen" option in the PHP-FPM-pool config.
status = "/status"                   # The value of the "status" option.
`
)

func init() {
	home := os.Getenv("HOME")

	if runtime.GOOS == "windows" {
		home = os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
	}

	p := path.Join(home, configFilename)
	_, err := toml.DecodeFile(p, &configs)

	if e, ok := err.(*os.PathError); ok && e.Err == syscall.ENOENT {
		// The configuration file was not found. Let's provide a sane
		// default for php5-fpm.
		configs = make(map[string]config)

		ioutil.WriteFile(p, []byte(defaultConfig), 0640)

		fmt.Printf("New configuration file written to \033[33m%s\033[0m. Please verify, and restart phpfpmtop.\n\n\033[33m%s\033[0m\n", p, defaultConfig)
		os.Exit(1)
	} else if err != nil {
		fmt.Printf("Error when opening %s: %s", p, err.Error())
		os.Exit(1)
	}
}

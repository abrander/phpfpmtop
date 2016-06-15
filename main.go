package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/BurntSushi/toml"
)

type (
	process struct {
		Pid               int       `json:"pid"`
		LastRequestCpu    float64   `json:"last request cpu"`
		LastRequestMemory int       `json:"last request memory"`
		State             string    `json:"state"`
		User              string    `json:"user"`
		ContentLength     int       `json:"content length"`
		RequestUri        string    `json:"request uri"`
		RequestDuration   int       `json:"request duration"`
		Requests          int       `json:"requests"`
		StartSince        int       `json:"start since"`
		StartTime         time.Time `json:"start time_FIXME"`
		Script            string    `json:"script"`
		RequestMethod     string    `json:"request method"`
	}

	status struct {
		SlowRequests        int       `json:"slow requests"`
		AcceptedConnections int       `json:"accepted conn"`
		TotalProcesses      int       `json:"total processes"`
		ListenQueue         int       `json:"listen queue"`
		IdleProcesses       int       `json:"idle processes"`
		Processes           []process `json:"processes"`
		MaxActiveProcesses  int       `json:"max active processes"`
		ActiveProcesses     int       `json:"active processes"`
		MaxListenQueue      int       `json:"max listen queue"`
		StartSince          int       `json:"start since"`
		StartTime           time.Time `json:"start time_FIXME"`
		ProcessManager      string    `json:"process manager"`
		MaxChildrenReached  int       `json:"max children reached"`
		Pool                string    `json:"pool"`
	}

	frame struct {
		Record  record
		Content []byte
	}

	record struct {
		Version       byte
		Type          byte
		RequestID     uint16
		ContentLength uint16
		PaddingLength byte
		Reserved      byte
	}

	appRecord struct {
		Role     uint16
		Flags    byte
		Reserved [5]byte
	}

	winsize struct {
		Rows, Columns  uint16
		XPixel, YPixel uint16
	}

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

func getTerminalSize() (int, int) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		log.Fatal(err)
	}
	defer tty.Close()

	ttyFd := tty.Fd()

	ws := winsize{}

	syscall.Syscall(syscall.SYS_IOCTL,
		ttyFd, uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)))

	return int(ws.Columns), int(ws.Rows)
}

func gather(conf config, s *status) error {
	var network string

	if strings.HasPrefix(conf.ListenPath, "/") {
		network = "unix"
	} else {
		network = "tcp"
	}

	conn, err := net.Dial(network, conf.ListenPath)
	if err != nil {
		return err
	}

	// We implement just enough of FastCGI to "GET" the status page. Nothing
	// more. It will probably break in exciting ways.

	// {FCGI_BEGIN_REQUEST, 1, {FCGI_RESPONDER, 0}}
	app := appRecord{
		Role: 1, // FCGI_RESPONDER
	}

	r := record{
		Version:       1,
		Type:          1, // FCGI_BEGIN_REQUEST
		ContentLength: uint16(binary.Size(app)),
	}

	err = binary.Write(conn, binary.BigEndian, r)
	if err != nil {
		return err
	}

	err = binary.Write(conn, binary.BigEndian, app)
	if err != nil {
		return err
	}

	// {FCGI_PARAMS, 1, "\013\002SERVER_PORT80" "\013\016SERVER_ADDR199.170.183.42 ... "}
	p := NewParams()

	p["SCRIPT_NAME"] = conf.StatusPath
	p["SCRIPT_FILENAME"] = conf.StatusPath
	p["REQUEST_METHOD"] = "GET"
	p["QUERY_STRING"] = "full&json& (phpfpmtop)"

	r = record{
		Version:       1,
		Type:          4, // FCGI_PARAMS
		ContentLength: p.Size(),
	}

	err = binary.Write(conn, binary.BigEndian, r)
	if err != nil {
		return err
	}

	err = p.Write(conn)
	if err != nil {
		return err
	}

	// {FCGI_PARAMS, 1, ""}
	r = record{
		Version: 1,
		Type:    4, // FCGI_PARAMS
	}
	err = binary.Write(conn, binary.BigEndian, r)
	if err != nil {
		return err
	}

	// {FCGI_STDIN, 1, ""}
	r = record{
		Version: 1,
		Type:    5, // FCGI_STDIN
	}
	err = binary.Write(conn, binary.BigEndian, r)
	if err != nil {
		return err
	}

	var stdin []byte
	var stderr []byte

	for {
		var f *frame
		f, err = readFrame(conn)

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		// Collect stdin
		if f.Record.Type == 6 {
			stdin = append(stdin, f.Content...)
		}

		// Collect stderr
		if f.Record.Type == 7 {
			stderr = append(stderr, f.Content...)
		}
	}

	if len(stderr) > 0 {
		return fmt.Errorf("Could not get '%s': %s", conf.StatusPath, string(stderr))
	}

	if len(stdin) > 0 {
		start := strings.IndexRune(string(stdin), '{')
		err = json.Unmarshal(stdin[start:], s)
		if err != nil {
			return err
		}
	}

	err = conn.Close()
	if err != nil {
		return err
	}

	return nil
}

// readFrame will read a single frame including padding from a FastCGI peer.
func readFrame(conn io.Reader) (*frame, error) {
	var r frame
	// Read reply
	err := binary.Read(conn, binary.BigEndian, &r.Record)
	if err != nil {
		return nil, err
	}

	if r.Record.ContentLength > 0 {
		r.Content = make([]byte, r.Record.ContentLength)
		n, err := io.ReadFull(conn, r.Content)
		if err != nil {
			return nil, err
		}

		if n != int(r.Record.ContentLength) {
			return nil, fmt.Errorf("Short read. Got %d, expected %d", n, r.Record.ContentLength)
		}
	}

	if r.Record.PaddingLength > 0 {
		buf := make([]byte, r.Record.PaddingLength)
		n, err := io.ReadFull(conn, buf)
		if err != nil {
			return nil, err
		}
		if n != int(r.Record.PaddingLength) {
			return nil, fmt.Errorf("Short read. Got %d, expected %d", n, r.Record.PaddingLength)
		}
	}

	return &r, nil
}

func main() {
	selectedConfig := "default"
	if len(os.Args) > 1 {
		selectedConfig = os.Args[1]
	}

	conf, found := configs[selectedConfig]
	if !found {
		fmt.Printf("%s not found in config file. Please fix.\n", selectedConfig)
		os.Exit(1)
	}

	last := status{}
	gather(conf, &last)

	line := NewSparkRing(70)

	s := status{}
	lastTime := time.Now().Add(-time.Second)

	// SHOW CURSOR: fmt.Printf("\033[?25l")

	// Hide cursor and clear screen.
	fmt.Printf("\033[?25l\033[2J")

	for t := range time.Tick(time.Millisecond * 250) {
		err := gather(conf, &s)
		if err != nil {
			fmt.Printf("\033[H\033[2JError: %s", err.Error())

			continue
		}

		_, height := getTerminalSize()
		delta := t.Sub(lastTime)

		sort.Sort(processSort(s.Processes))

		uptime := time.Second * time.Duration(s.StartSince)

		requestPerSecond := float64(s.AcceptedConnections-last.AcceptedConnections) / (float64(delta) / float64(time.Second))

		// Draw the sparkline showing request per second.
		line.Push(requestPerSecond)

		// Start in the upper left.
		fmt.Printf("\033[0;0H")

		// Print headers.
		fmt.Printf("PHP-FPM Pool: \033[32m%s\033[0m   Uptime: \033[32m%s\033[0m   Manager: \033[32m%s\033[0m   Accepted Connections: \033[32m%d\033[0m\033[K\n", s.Pool, uptime.String(), s.ProcessManager, s.AcceptedConnections)
		fmt.Printf("Active/Total: \033[32m%4d\033[0m/\033[32m%-4d\033[0m   Queue: \033[32m%d\033[0m   Request per Second: \033[32m%.1f\033[0m\033[K\n", s.ActiveProcesses, s.ActiveProcesses+s.IdleProcesses, s.ListenQueue, requestPerSecond)

		// Print beautiful sky colored table headers.
		fmt.Printf("%s\033[K\n\033[0;37;44m", line.String())
		fmt.Printf("%7s %10s %10s %10s %10s", "PID", "Uptime", "State", "Mem", "Duration")
		fmt.Printf("\033[K\033[0m")

		// Make room for headers.
		height -= 3

		for _, pro := range s.Processes {
			height--
			if height == 0 {
				break
			}

			fmt.Printf("\n")

			// Print a single square showing state.
			switch pro.State {
			case "Running":
				fmt.Printf("\033[45m \033[0m")
			case "Idle":
				fmt.Printf("\033[42m \033[0m")
			default:
				fmt.Printf("\033[41m \033[0m")
			}

			dur := time.Microsecond * time.Duration(pro.RequestDuration)
			up := time.Second * time.Duration(pro.StartSince)

			// Print running processes in bold.
			if pro.State == "Running" {
				fmt.Printf("\033[1m")
			}

			// If the duration is more than 500ms, print in yellow.
			if dur > time.Millisecond*500 {
				// Or red if we exceed 1 second.
				if dur > time.Millisecond*1000 {
					fmt.Printf("\033[31m")
				} else {
					fmt.Printf("\033[33m")
				}
			}

			// Print the process line.
			fmt.Printf("%7d %10s %10s %10d %10s %7s %s\033[K", pro.Pid, up.String(), pro.State, pro.LastRequestMemory, dur.String(), pro.RequestMethod, pro.RequestUri)

			// Rerset ANSI colors etc.
			fmt.Printf("\033[0m")
		}

		lastTime = t
		last = s
	}
}

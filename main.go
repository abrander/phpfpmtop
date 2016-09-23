package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/pkg/term"
)

type (
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
)

var (
	updateDelays = []time.Duration{
		time.Millisecond * 50,
		time.Millisecond * 100,
		time.Millisecond * 250,
		time.Millisecond * 500,
		time.Second,
		time.Second * 2,
		time.Second * 5,
		time.Second * 10,
		time.Second * 30,
		time.Second * 60,
		time.Hour,
		time.Hour * 24}
)

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

func fpmGet(listenPath string, path string) ([]byte, error) {
	var network string

	if strings.HasPrefix(listenPath, "/") {
		network = "unix"
	} else {
		network = "tcp"
	}

	conn, err := net.Dial(network, listenPath)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

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
		return nil, err
	}

	err = binary.Write(conn, binary.BigEndian, app)
	if err != nil {
		return nil, err
	}

	// {FCGI_PARAMS, 1, "\013\002SERVER_PORT80" "\013\016SERVER_ADDR199.170.183.42 ... "}
	p := NewParams()

	p["SCRIPT_NAME"] = path
	p["SCRIPT_FILENAME"] = path
	p["REQUEST_METHOD"] = "GET"
	p["QUERY_STRING"] = "full&json& (phpfpmtop)"

	r = record{
		Version:       1,
		Type:          4, // FCGI_PARAMS
		ContentLength: p.Size(),
	}

	err = binary.Write(conn, binary.BigEndian, r)
	if err != nil {
		return nil, err
	}

	err = p.Write(conn)
	if err != nil {
		return nil, err
	}

	// {FCGI_PARAMS, 1, ""}
	r = record{
		Version: 1,
		Type:    4, // FCGI_PARAMS
	}
	err = binary.Write(conn, binary.BigEndian, r)
	if err != nil {
		return nil, err
	}

	// {FCGI_STDIN, 1, ""}
	r = record{
		Version: 1,
		Type:    5, // FCGI_STDIN
	}
	err = binary.Write(conn, binary.BigEndian, r)
	if err != nil {
		return nil, err
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
			return nil, err
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
		return nil, fmt.Errorf("Could not get '%s': %s", path, string(stderr))
	}

	return stdin, nil
}

func gather(conf config, s *status) error {
	body, err := fpmGet(conf.ListenPath, conf.StatusPath)

	if len(body) > 0 {
		start := strings.IndexRune(string(body), '{')
		err = json.Unmarshal(body[start:], s)
		if err != nil {
			return err
		}
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

	t, _ := term.Open("/dev/tty")
	term.CBreakMode(t)

	keyboard := make(chan rune)
	// Read from keyboard.
	go func() {
		bytes := make([]byte, 3)
		for {
			numRead, _ := t.Read(bytes)

			for _, key := range bytes[:numRead] {
				keyboard <- rune(key)
			}
		}
	}()

	last := status{}
	gather(conf, &last)

	line := NewSparkRing(70)

	s := status{}
	lastTime := time.Now().Add(-time.Second)

	// SHOW CURSOR: fmt.Printf("\033[?25l")

	// Hide cursor and clear screen.
	fmt.Printf("\033[?25l\033[2J")

	timer := time.NewTimer(time.Duration(0))

	// Catch signals from OS or shell.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	signal.Notify(quit, syscall.SIGTERM)

	delay := 1
	lessDelay := func() {
		delay--
		if delay < 0 {
			delay = 0
		}

		timer.Reset(0)
	}

	moreDelay := func() {
		delay++

		if delay > len(updateDelays)-1 {
			delay = len(updateDelays) - 1
		}

		timer.Reset(0)
	}

MAINLOOP:
	for {
		select {
		case <-quit:
			break MAINLOOP

		case key := <-keyboard:
			switch key {
			case 'q':
				break MAINLOOP
			case '-':
				lessDelay()
			case '+':
				moreDelay()
			case ' ':
				// We trigger the timer now to redraw at once.
				timer.Reset(0)
			}

		case t := <-timer.C:
			err := gather(conf, &s)
			if err != nil {
				fmt.Printf("\033[H\033[2JError: %s", err.Error())

				timer.Reset(updateDelays[delay] - time.Now().Sub(t))
				continue MAINLOOP
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
			fmt.Printf("PHP-FPM Pool: \033[32m%s\033[0m   Uptime: \033[32m%s\033[0m   Manager: \033[32m%s\033[0m   Accepted Connections: \033[32m%d\033[0m\033[K\n\r", s.Pool, uptime.String(), s.ProcessManager, s.AcceptedConnections)
			fmt.Printf("Active/Total: \033[32m%4d\033[0m/\033[32m%-4d\033[0m   Queue: \033[32m%d\033[0m   Request per Second: \033[32m%.1f\033[0m\033[K   Poll Delay: \033[32m%s\033[0m\n\r", s.ActiveProcesses, s.ActiveProcesses+s.IdleProcesses, s.ListenQueue, requestPerSecond, updateDelays[delay].String())

			// Print beautiful sky colored table headers.
			fmt.Printf("%s\033[K\n\r\033[0;37;44m", line.String())
			fmt.Printf("%7s %10s %10s %10s %10s", "PID", "Uptime", "State", "Mem", "Duration")
			fmt.Printf("\033[K\033[0m")

			// Make room for headers.
			height -= 3

			for _, pro := range s.Processes {
				height--
				if height == 0 {
					break
				}

				fmt.Printf("\n\r")

				// Print a single square showing state.
				switch pro.State {
				case Running:
					fmt.Printf("\033[45m \033[0m")
				case Idle:
					fmt.Printf("\033[42m \033[0m")
				default:
					fmt.Printf("\033[41m \033[0m")
				}

				dur := time.Microsecond * time.Duration(pro.RequestDuration)
				up := time.Second * time.Duration(pro.StartSince)

				// Print running processes in bold.
				if pro.State == Running {
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
				fmt.Printf("%7d %10s %10s %10d %10s %7s %s\033[K", pro.Pid, up.String(), pro.State, pro.LastRequestMemory, dur.String(), pro.RequestMethod, pro.RequestURI)

				// Rerset ANSI colors etc.
				fmt.Printf("\033[0m")
			}

			lastTime = t
			last = s

			timer.Reset(updateDelays[delay] - time.Now().Sub(t))
		}
	}

	// Enable cursor and restore terminal.
	fmt.Printf("\033[?25h\n\r")

	t.Restore()
	t.Close()
}

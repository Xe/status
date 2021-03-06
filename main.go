package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/tv42/httpunix"
)

var (
	daemonFlag = flag.Bool("d", false, "run as HTTP daemon?")

	ctrlSocketLoc = flag.String("socloc", "", "custom control socket location")
	fifoLoc       = flag.String("fifoloc", "", "custom fifo folder location")

	message string

	netDevs = map[string]struct{}{
		"eth0:":  {},
		"eth1:":  {},
		"wlan0:": {},
		"ppp0:":  {},
	}
	cores = runtime.NumCPU() // count of cores to scale cpu usage
	rxOld = 0
	txOld = 0
)

// fixed builds a fixed width string with given pre- and fitting suffix
func fixed(pre string, rate int) string {
	if rate < 0 {
		return pre + " ERR"
	}

	var spd = float32(rate)
	var suf = "B/s" // default: display as B/s

	switch {
	case spd >= (1000 * 1024 * 1024): // > 999 MiB/s
		return "" + pre + "ERR"
	case spd >= (1000 * 1024): // display as MiB/s
		spd /= (1024 * 1024)
		suf = "MiB/s"
		pre = "" + pre + ""
	case spd >= 1000: // display as KiB/s
		spd /= 1024
		suf = "KiB/s"
	}

	var formated = ""
	if spd >= 100 {
		formated = fmt.Sprintf("%3.0f", spd)
	} else if spd >= 10 {
		formated = fmt.Sprintf("%4.1f", spd)
	} else {
		formated = fmt.Sprintf(" %3.1f", spd)
	}
	return pre + formated + suf
}

// colored surrounds the percentage with color escapes if it is >= 70
func colored(icon string, percentage int) string {
	if percentage >= 100 {
		return fmt.Sprintf("%s %3d", icon, percentage)
	} else if percentage >= 70 {
		return fmt.Sprintf("%s %3d", icon, percentage)
	}
	return fmt.Sprintf("%s%3d", icon, percentage)
}

// updateCPUUse reads the last minute sysload and scales it to the core count
func updateCPUUse() string {
	var load float32
	var loadavg, err = ioutil.ReadFile("/proc/loadavg")
	if err != nil {
		return "cpu ERR"
	}
	_, err = fmt.Sscanf(string(loadavg), "%f", &load)
	if err != nil {
		return "cpu ERR"
	}
	return colored("cpu", int(load*100.0/float32(cores)))
}

// updateMemUse reads the memory used by applications and scales to [0, 100]
func updateMemUse() string {
	var file, err = os.Open("/proc/meminfo")
	if err != nil {
		return "ram ERR"
	}
	defer file.Close()

	// done must equal the flag combination (0001 | 0010 | 0100 | 1000) = 15
	var total, used, done = 0, 0, 0
	for info := bufio.NewScanner(file); done != 15 && info.Scan(); {
		var prop, val = "", 0
		if _, err = fmt.Sscanf(info.Text(), "%s %d", &prop, &val); err != nil {
			return "ram ERR"
		}
		switch prop {
		case "MemTotal:":
			total = val
			used += val
			done |= 1
		case "MemFree:":
			used -= val
			done |= 2
		case "Buffers:":
			used -= val
			done |= 4
		case "Cached:":
			used -= val
			done |= 8
		}
	}
	return colored("ram", used*100/total)
}

func main() {
	flag.Parse()

	if *daemonFlag {
		go func() {
			for {
				var status []string

				if message != "" {
					status = append(status, message)
				}

				status = append(status,
					[]string{
						updateCPUUse(),
						updateMemUse(),
						time.Now().Local().Format("Mon 02 15:04"),
					}...,
				)

				hostname, _ := os.Hostname()

				mymessage := strings.Join(status, " | ")
				exec.Command("xsetroot", "-name", hostname+" | "+mymessage).Run()

				files, err := ioutil.ReadDir("/home/xena/.local/share/within/status/fifos")
				if err != nil {
					log.Fatal("wtf")
				}

				for _, file := range files {
					if file.IsDir() {
						continue
					}

					fout, err := os.Create("/home/xena/.local/share/within/status/fifos/" + file.Name())
					if err != nil {
						log.Printf("Could not report to %s?", file.Name())
					}

					_, err = fout.Write(append([]byte(mymessage), []byte("\n")...))
					if err != nil {
						panic(err)
					}
				}

				// sleep until beginning of next second
				var now = time.Now()
				time.Sleep(now.Truncate(time.Second).Add(time.Second).Sub(now))
			}
		}()

		mux := http.NewServeMux()

		mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
			if req.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				fmt.Fprintf(w, "Use POST")
				return
			}

			data, err := ioutil.ReadAll(req.Body)
			if err != nil {
				fmt.Fprintf(w, "%s", err)
			}

			message = string(data)
		})

		s := &http.Server{
			Handler: mux,
		}

		os.Remove("/home/xena/.local/share/within/status/status.sock")

		l, err := net.Listen("unix", "/home/xena/.local/share/within/status/status.sock")
		if err != nil {
			log.Fatal(err)
		}

		defer os.Remove("/home/xena/.local/share/within/status/status.sock")

		err = s.Serve(l)
		if err != nil {
			log.Fatal(err)
		}

		os.Exit(0)
	}

	message := []byte(strings.Join(flag.Args(), " "))
	buf := bytes.NewBuffer(message)

	u := &httpunix.Transport{
		DialTimeout:           100 * time.Millisecond,
		RequestTimeout:        1 * time.Second,
		ResponseHeaderTimeout: 1 * time.Second,
	}
	u.RegisterLocation("status", "/home/xena/.local/share/within/status/status.sock")

	var client = http.Client{
		Transport: u,
	}

	resp, err := client.Post("http+unix://status/", "text/plain", buf)
	if err != nil {
		log.Fatal(err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Fatal(resp.Status)
	}
}

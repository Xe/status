package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tv42/httpunix"
)

var (
	daemonFlag = flag.Bool("d", false, "run as HTTP daemon?")

	ctrlSocketLoc = flag.String("socloc", "", "custom control socket location")
	fifoLoc       = flag.String("fifoloc", "", "custom fifo folder location")
)

func main() {
	flag.Parse()

	if *daemonFlag {
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

			files, err := ioutil.ReadDir("/home/xena/.local/share/within/status/fifos")
			if err != nil {
				w.WriteHeader(500)
				fmt.Fprintf(w, "Error: %v", err)
				return
			}

			for _, file := range files {
				if file.IsDir() {
					continue
				}

				fout, err := os.Create("/home/xena/.local/share/within/status/fifos/" + file.Name())
				if err != nil {
					log.Printf("Could not report to %s?", file.Name())
				}

				_, err = fout.Write(append(data, []byte("\n")...))
				if err != nil {
					panic(err)
				}
			}
		})

		s := &http.Server{
			Handler: mux,
		}

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

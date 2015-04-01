package main

import (
	"./format"
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Tick struct {
	Size    int64
	Latency time.Duration
}

type Config struct {
	Url         string
	Duration    time.Duration
	Threads     int
	Connections int
	addr        *net.TCPAddr
	url         *url.URL
}

type Errors struct {
	connect uint64
	read    uint64
	write   uint64
	status  uint64
	timeout uint64
}

type Thread struct {
	url            *url.URL
	addr           *net.TCPAddr
	conn           *net.TCPConn
	complete       uint64
	requests       uint64
	bytes          uint64
	start          time.Time
	latency        time.Duration
	connectLatency time.Duration
	errors         Errors
	quit           chan bool
}

func usage() {
	fmt.Println("Usage: http-bench <options> <url>\n  Options:\n    -c, --connections <N>  Connections to keep open\n    -d, --duration    <T>  Duration of test")
}

func main() {
	config := Config{}
	flag.DurationVar(&config.Duration, "d", 10, "Duration of test")
	flag.IntVar(&config.Threads, "t", 10, "Number of threads to use")
	flag.Parse()

	if len(flag.Args()) != 1 {
		usage()
		return
	}

	config.Url = flag.Args()[0]
	if config.Url == "" {
		usage()
		return
	}

	config.Connections = config.Threads

	start(config)
}

func start(config Config) {
	fmt.Printf("Running %v test @ %v\n", config.Duration, config.Url)
	fmt.Printf("  %v threads and %v connections\n", config.Threads, config.Connections)

	resolveAddr(&config)

	threads := make([]*Thread, 0)
	for i := 0; i < config.Threads; i++ {
		thread := Thread{addr: config.addr, url: config.url}
		startWorker(&config, &thread)
		threads = append(threads, &thread)
	}

	for i := 0; i < config.Threads; i++ {
		<-threads[i].quit
	}

	results := mergeResults(threads)
	outputResult(results)
}

func mergeResults(threads []*Thread) *Thread {
	return threads[0]
}

func outputResult(t *Thread) {
	var avg time.Duration
	var reqps, bytesps float64
	if t.complete > 0 {
		avg = (time.Duration)(int64(t.latency) / int64(t.complete))
		reqps = float64(time.Second) / float64(avg)
		bytesps = float64(t.bytes) / float64(float64(t.latency)/float64(time.Second))
	}

	fmt.Println("Latency:", format.Duration(avg))
	fmt.Printf("%v requests in %v, %v read\n", t.complete, format.Duration(t.latency), format.Bytes(float64(t.bytes)))
	fmt.Printf("Requests/sec: %v\n", format.Reqps(reqps))
	fmt.Printf("Transfer/sec: %v\n", format.Bytes(bytesps))
	fmt.Printf("Errors: %v connect, %v write, %v read, %v status, %v timeout\n", t.errors.connect, t.errors.write, t.errors.read, t.errors.status, t.errors.timeout)
	fmt.Printf("%v\n", t)
}

func startWorker(config *Config, thread *Thread) {
	thread.quit = make(chan bool)

	go func() {
		timeout_at := time.After(config.Duration)
		for {
			select {
			case <-timeout_at:
				thread.quit <- true
				return
			default:
				connect(thread)
			}
		}
	}()
}

func resolveAddr(config *Config) {
	var err error
	config.url, err = url.Parse(config.Url)
	if err != nil {
		log.Fatalln(err.Error())
	}

	if config.url.Path == "" {
		config.url.Path = "/"
	}

	host := config.url.Host
	if !strings.Contains(host, ":") {
		switch config.url.Scheme {
		case "https":
			host = fmt.Sprintf("%s:%v", host, 443)
		case "http":
			host = fmt.Sprintf("%s:%v", host, 80)
		default:
			errMsg := fmt.Sprintf("Unknown scheme %v", config.url.Scheme)
			log.Fatalln(errors.New(errMsg))
		}
	}

	config.addr, err = net.ResolveTCPAddr("tcp", host)
	if err != nil {
		log.Fatalln(err.Error())
	}
}

func connect(t *Thread) {
	var err error
	t.start = time.Now()

	if t.conn != nil {
		post_request(t)
		return
	}

	fmt.Println("Dialing tcp connection")
	t.conn, err = net.DialTCP("tcp", nil, t.addr)
	if err == nil {
		post_request(t)
	} else {
		t.errors.connect++
	}
}

func post_request(t *Thread) {
	req := fmt.Sprintf("GET %v HTTP/1.1\r\nHost: %v\r\nConnection: Keep-Alive\r\n\r\n", t.url.Path, t.url.Host)
	_, err := fmt.Fprintf(t.conn, "%s", req)
	if err != nil {
		t.errors.write++
		t.conn.Close()
		t.conn = nil
	}

	r := bufio.NewReader(t.conn)
	status, err := r.ReadString('\n')

	// Read status
	if err != nil {
		t.errors.read++
		t.conn.Close()
		t.conn = nil
		return
	}

	ok := strings.Index(status, "HTTP/1.1 200") == 0
	if !ok {
		t.errors.status++
	}

	t.bytes += uint64(len(status))

	// Read headers
	var s string
	var content_length uint64
	keep_alive := true

	for {
		s, err = r.ReadString('\n')
		if err != nil {
			t.errors.read++
			t.conn.Close()
			t.conn = nil
			return
		}

		t.bytes += uint64(len(s))

		headers := strings.SplitN(s, ":", 2)
		switch strings.ToLower(headers[0]) {
		case "content-length":
			trimmed := strings.Trim(headers[1], " \r\n")
			content_length, err = strconv.ParseUint(trimmed, 10, 64)
			if err != nil {
				log.Fatalln(err.Error())
			}
		case "connection":
			keep_alive = strings.Contains(strings.ToLower(headers[1]), "keep-alive")
		}

		if s == "\r\n" {
			break
		}
	}

	// Ready body.
	buf := make([]byte, 1024)
	var n int
	for {
		n, err = r.Read(buf)
		if err != nil {
			t.errors.read++
			t.conn.Close()
			t.conn = nil
			return
		}
		if n > 0 {
			content_length -= uint64(n)
		}
		if content_length == 0 {
			break
		}
	}

	if n > 0 {
		t.bytes += uint64(n)
	}

	// Completed.
	t.complete++
	t.latency += time.Since(t.start)

	if !keep_alive {
		t.conn.Close()
		t.conn = nil
	}
}

package main

import (
	"./format"
	"bufio"
	"crypto/tls"
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
	tlsConn        *tls.Conn
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

	start(config)
}

func start(config Config) {
	fmt.Printf("Running %v test @ %v using %v threads.\n", config.Duration, config.Url, config.Threads)

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

	format.Errors(t.errors.connect, "connect")
	format.Errors(t.errors.write, "write")
	format.Errors(t.errors.read, "read")
	format.Errors(t.errors.status, "status")
	format.Errors(t.errors.timeout, "timeout")
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

	if t.conn != nil {
		post_request(t)
		return
	}

	t.conn, err = net.DialTCP("tcp", nil, t.addr)
	if err != nil {
		t.errors.connect++
		resetConnection(t)
		return
	}

	if t.url.Scheme == "https" {
		t.tlsConn = tls.Client(t.conn, &tls.Config{InsecureSkipVerify: true})
		err = t.tlsConn.Handshake()
		if err != nil {
			log.Fatalln(err.Error())
		}
	}

	post_request(t)
}

func post_request(t *Thread) {
	req := fmt.Sprint("GET ", t.url.Path, " HTTP/1.1\r\nHost: ", t.url.Host,
		"\r\nUser-Agent: http-bench\r\n\r\n")

	t.start = time.Now()
	var err error
	if t.tlsConn != nil {
		_, err = fmt.Fprint(t.tlsConn, req)
	} else {
		_, err = fmt.Fprint(t.conn, req)
	}

	if err != nil {
		t.errors.write++
		resetConnection(t)
	}

	read_response(t)
}

func read_response(t *Thread) {
	var r *bufio.Reader
	if t.tlsConn != nil {
		r = bufio.NewReader(t.tlsConn)
	} else {
		r = bufio.NewReader(t.conn)
	}

	status, err := r.ReadString('\n')

	// Read status
	if err != nil {
		t.errors.read++
		resetConnection(t)
		return
	}

	statusWords := strings.Split(status, " ")
	if len(statusWords) < 3 {
		t.errors.read++
		resetConnection(t)
		return
	}

	statusCode, err := strconv.ParseUint(statusWords[1], 10, 64)
	if err != nil {
		log.Fatalln("Cannot parse status code:", status)
	}

	if statusCode > 399 {
		fmt.Println(status)
		t.errors.status++

		if statusCode == 400 {
			log.Fatalln(status)
		}
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
			resetConnection(t)
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

	// Read body.
	buf := make([]byte, content_length)
	remains := content_length
	for {
		var n int
		n, err = r.Read(buf)
		if err != nil {
			t.errors.read++
			resetConnection(t)
			return
		}

		if n > 0 {
			remains -= uint64(n)
		}

		if remains <= 0 {
			break
		}
	}

	// Completed
	t.latency += time.Since(t.start)
	t.bytes += uint64(content_length)
	t.complete++

	if !keep_alive {
		resetConnection(t)
	}
}

func resetConnection(t *Thread) {
	if t.tlsConn != nil {
		t.tlsConn.Close()
		t.tlsConn = nil
	}
	t.conn.Close()
	t.conn = nil
}

package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/http/httputil"
	"time"
)

type Tick struct {
	Size    int64
	Latency time.Duration
}

var (
	// Concurrency int
	Url      string
	Duration time.Duration
	// Threads  = 8
	// Connections = 10
	// Timeout = 500 * time.Millisecond
)

func usage() {
	fmt.Println("Usage: http-bench <options> <url>\n  Options:\n    -c, --connections <N>  Connections to keep open\n    -d, --duration    <T>  Duration of test")
}

func main() {
	flag.DurationVar(&Duration, "d", 10*time.Second, "Duration")
	// Duration = *flag.Duration("d", 10*time.Second, "Duration")
	flag.Parse()

	if len(flag.Args()) != 1 {
		usage()
		return
	}

	Url = flag.Args()[0]
	if Url == "" {
		usage()
		return
	}

	start(Url, Duration)
}

func start(url string, duration time.Duration) {
	fmt.Println("Running", duration, "test @", url)

	timeout_at := time.After(duration)
	chquit, chtick := startWorker(url)

	ticks := make([]Tick, 0)

	for {
		select {
		case tick := <-chtick:
			ticks = append(ticks, tick)
		case <-timeout_at:
			chquit <- true
			outputResult(&ticks)
			// panic("wow")
			return
		}
	}
}

func outputResult(ticks *[]Tick) {
	total_latency := time.Millisecond
	var total_size int64
	for _, v := range *ticks {
		total_latency += v.Latency
		total_size += v.Size
	}
	ticks_count := len(*ticks)
	avg := (time.Duration)(int64(total_latency) / int64(ticks_count))
	reqps := float64(time.Second) / float64(avg)
	bytesps := float64(total_size) / float64(float64(total_latency)/float64(time.Second))
	fmt.Println("Latency:", avg)

	// fmt.Println("Total requests:", ticks_count)
	// fmt.Println("Time worked:", total_latency)
	// fmt.Println("Bytes read:", total_size)

	// fmt.Println()

	fmt.Printf("%v requests in %v, %v bytes read\n", ticks_count, total_latency, total_size)
	fmt.Printf("Requests/sec: %.2f\n", reqps)
	fmt.Printf("Transfer/sec: %v\n", formatBytes(bytesps))
	// 71 requests in 1.04s, 43.82KB read
	// Requests/sec:     68.37
	// Transfer/sec:     42.20KB
}

var bytePowers = map[string]float64{
	"TB": math.Pow(1024, 4),
	"GB": math.Pow(1024, 3),
	"MB": math.Pow(1024, 2),
	"KB": math.Pow(1024, 1),
	"B":  math.Pow(1024, 0),
}

func formatBytes(bytes float64) string {
	limit := 2.61803398875 * bytes
	for name, value := range bytePowers {
		if limit > value {
			return fmt.Sprintf("%.2f%v", bytes/value, name)
		}
	}

	return fmt.Sprintf("%vB", bytes)
}

func startWorker(url string) (chan bool, chan Tick) {
	chquit := make(chan bool, 1)
	chtick := make(chan Tick)
	go func() {
		client := &http.Client{
			Timeout: 60 * time.Second,
		}
		for {
			select {
			case <-chquit:
				fmt.Println("chquit received")
				return
			default:
				chtick <- measureUrl(client, url)
			}
		}
	}()

	return chquit, chtick
}

func measureUrl(client *http.Client, url string) Tick {
	started := time.Now()
	resp, err := client.Get(url)
	duration := time.Since(started)
	defer resp.Body.Close()

	if err != nil {
		log.Fatalln(err.Error())
	}

	if resp.StatusCode != 200 {
		log.Fatalln("Status is not 200:", resp)
	}

	var data []byte

	data, err = httputil.DumpResponse(resp, true)
	data_length := int64(len(data))

	return Tick{Latency: duration, Size: data_length}
}

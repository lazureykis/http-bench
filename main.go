package main

import (
	"./format"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"time"
)

type Tick struct {
	Size    int64
	Latency time.Duration
}

type Config struct {
	Url      string
	Duration time.Duration
	Threads  int
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
	fmt.Println("Running", config.Duration, "test @", config.Url)

	chresult := startWorker(config)

	ticks := <-chresult
	outputResult(&ticks)
}

func outputResult(ticks *[]Tick) {
	total_latency := time.Millisecond
	var total_size int64
	for _, v := range *ticks {
		total_latency += v.Latency
		total_size += v.Size
	}

	ticks_count := len(*ticks)
	var avg time.Duration
	var reqps, bytesps float64
	if ticks_count > 0 {
		avg = (time.Duration)(int64(total_latency) / int64(ticks_count))
		reqps = float64(time.Second) / float64(avg)
		bytesps = float64(total_size) / float64(float64(total_latency)/float64(time.Second))
	}

	fmt.Println("Latency:", format.Duration(avg))
	fmt.Printf("%v requests in %v, %v read\n", ticks_count, format.Duration(total_latency), format.Bytes(float64(total_size)))
	fmt.Printf("Requests/sec: %v\n", format.Reqps(reqps))
	fmt.Printf("Transfer/sec: %v\n", format.Bytes(bytesps))
}

func startWorker(config Config) chan []Tick {
	chresult := make(chan []Tick)

	go func() {
		client := &http.Client{
			Timeout: 60 * time.Second,
		}
		ticks := make([]Tick, 0)
		timeout_at := time.After(config.Duration)

		for {
			select {
			case <-timeout_at:
				chresult <- ticks
				return
			default:
				ticks = append(ticks, measureUrl(client, config.Url))
			}
		}
	}()

	return chresult
}

func measureUrl(client *http.Client, url string) Tick {
	started := time.Now()
	resp, err := client.Get(url)
	duration := time.Since(started)
	defer resp.Body.Close()

	if err != nil {
		log.Fatalln(err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		log.Fatalln("Status is not 200:", resp)
	}

	var data []byte

	data, err = httputil.DumpResponse(resp, true)
	data_length := int64(len(data))

	return Tick{Latency: duration, Size: data_length}
}

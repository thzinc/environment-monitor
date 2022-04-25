package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/syncromatics/go-kit/cmd"
	"github.com/tarm/serial"
)

var (
	path        string = "/dev/ttyAMA0"
	baud        int    = 9600
	metricsPort int    = 9100

	opsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "xxx_tmp_ops_processed",
		Help: "The total number of processed events",
	})
)

func main() {
	fmt.Println("wow")
	config := &serial.Config{Name: path, Baud: baud, StopBits: 1, Parity: serial.ParityNone, Size: 8}
	port, err := serial.OpenPort(config)
	if err != nil {
		panic(err)
	}

	group := cmd.NewProcessGroup(context.Background())
	group.Go(func() error {
		http.Handle("/metrics", promhttp.Handler())
		return http.ListenAndServe(fmt.Sprintf(":%d", metricsPort), nil)
	})
	group.Go(func() error {
		for {
			b := make([]byte, 1)
			cnt, err := port.Read(b)
			if err != nil {
				return err
			}
			log.Printf("got %d %x: %b; want %b\n", cnt, b, b, 0x42)
			if b[0] == 0x42 {
				discard := make([]byte, 31)
				io.ReadAtLeast(port, discard, 31)
				break
			}
		}
		for {
			opsProcessed.Inc()
			buf := make([]byte, 32)
			c, err := io.ReadAtLeast(port, buf, 32)

			if err != nil {
				return err
			}
			fmt.Printf("%d: %v\n", c, buf[0:c])
		}
	})

	err = group.Wait()
	if err != nil {
		panic(err)
	}
}

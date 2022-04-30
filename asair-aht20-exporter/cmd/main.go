package main

import (
	"asair-aht20-exporter/aht20"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/syncromatics/go-kit/v2/cmd"
	"github.com/syncromatics/go-kit/v2/log"
)

const (
	MetricsPort int   = 9100
	i2cAddr     uint8 = 0x38
	i2cBus      int   = 1
)

var (
	aht_received_packets = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "aht_received_packets",
		},
	)
	aht_relative_humidity = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "aht_relative_humidity",
			Help: "Percentage of relative humidity",
		},
	)
	aht_temperature = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "aht_temperature",
			Help: "Temperature in degrees Celsius",
		},
	)
)

func main() {
	group := cmd.NewProcessGroup(context.Background())

	metricServer := http.Server{
		Addr:    fmt.Sprintf(":%d", MetricsPort),
		Handler: nil,
	}
	log.Info("starting metrics server",
		"addr", metricServer.Addr)
	group.Go(func() error {
		http.Handle("/metrics", promhttp.Handler())
		return metricServer.ListenAndServe()
	})
	group.Go(func() error {
		<-group.Context().Done()
		log.Info("stopping metrics server")
		return metricServer.Close()
	})

	sensor := aht20.NewSensor(i2cAddr, i2cBus, 5*time.Second) // HACK: should be configurable
	log.Info("starting sensor",
		"sensor", sensor)
	group.Go(sensor.Start(group.Context()))
	group.Go(func() error {
		for {
			log.Debug("waiting for reading...")
			select {
			case reading, ok := <-sensor.Readings():
				if !ok {
					log.Debug("readings channel closed")
					return nil
				}

				log.Debug("received reading",
					"reading", reading)

				aht_received_packets.Inc()
				aht_relative_humidity.Set(float64(reading.Humidity))
				aht_temperature.Set(float64(reading.Temperature))
			case <-group.Context().Done():
				return nil
			}
		}
	})

	err := group.Wait()
	if err != nil {
		log.Fatal("failed to terminate cleanly",
			"err", err)
	}
}

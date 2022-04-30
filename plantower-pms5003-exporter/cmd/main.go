package main

import (
	"context"
	"fmt"
	"net/http"
	"plantower-pms5003-exporter/pms5003"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/syncromatics/go-kit/v2/cmd"
	"github.com/syncromatics/go-kit/v2/log"
)

const (
	DefaultMetricsPort      int           = 9100
	DefaultPortName         string        = "/dev/ttyAMA0"
	DefaultReconnectTimeout time.Duration = 1 * time.Second
)

var (
	pms_received_packets = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "pms_received_packets",
		},
	)

	pms_particulate_matter_standard = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pms_particulate_matter_standard",
			Help: "Micrograms per cubic meter, standard particle",
		},
		[]string{"microns"},
	)

	pms_particulate_matter_environmental = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pms_particulate_matter_environmental",
			Help: "Micrograms per cubic meter, adjusted for atmospheric environment",
		},
		[]string{"microns"},
	)

	pms_particle_counts = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pms_particle_counts",
			Help: "Number of particles with diameter beyond given number of microns in 0.1L of air",
		},
		[]string{"microns_lower_bound"},
	)
)

func main() {
	group := cmd.NewProcessGroup(context.Background())

	metricServer := http.Server{
		Addr:    fmt.Sprintf(":%d", DefaultMetricsPort), // TODO: read from settings
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

	reconnectTimeout, err := time.ParseDuration("1s") // TODO: read from settings
	if err != nil {
		log.Warn("failed to parse reconnect timeout duration; using default timeout",
			"err", err,
			"DefaultReconnectTimeout", DefaultReconnectTimeout)
		reconnectTimeout = DefaultReconnectTimeout
	}

	portName := DefaultPortName // TODO: read from settings

	sensor := pms5003.NewSensor(portName, reconnectTimeout)
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

				pms_received_packets.Inc()
				pms_particulate_matter_standard.WithLabelValues("1").Set(float64(reading.Pm10Std))
				pms_particulate_matter_standard.WithLabelValues("2.5").Set(float64(reading.Pm25Std))
				pms_particulate_matter_standard.WithLabelValues("10").Set(float64(reading.Pm100Std))
				pms_particulate_matter_environmental.WithLabelValues("1").Set(float64(reading.Pm10Env))
				pms_particulate_matter_environmental.WithLabelValues("2.5").Set(float64(reading.Pm25Env))
				pms_particulate_matter_environmental.WithLabelValues("10").Set(float64(reading.Pm100Env))
				pms_particle_counts.WithLabelValues("0.3").Set(float64(reading.Particles3um))
				pms_particle_counts.WithLabelValues("0.5").Set(float64(reading.Particles5um))
				pms_particle_counts.WithLabelValues("1").Set(float64(reading.Particles10um))
				pms_particle_counts.WithLabelValues("2.5").Set(float64(reading.Particles25um))
				pms_particle_counts.WithLabelValues("5").Set(float64(reading.Particles50um))
				pms_particle_counts.WithLabelValues("10").Set(float64(reading.Particles100um))
			case <-group.Context().Done():
				return nil
			}
		}
	})

	err = group.Wait()
	if err != nil {
		log.Fatal("failed to terminate cleanly",
			"err", err)
	}
}

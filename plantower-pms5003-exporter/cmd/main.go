package main

import (
	"context"
	"fmt"
	"net/http"
	"plantower-pms5003-exporter/pms5003"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/syncromatics/go-kit/v2/cmd"
	"github.com/syncromatics/go-kit/v2/log"
)

const (
	MetricsPort int = 9100
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
			Help: "micrograms per cubic meter, adjusted for atmospheric environment",
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

	group.Go(func() error {
		http.Handle("/metrics", promhttp.Handler())
		return http.ListenAndServe(fmt.Sprintf(":%d", MetricsPort), nil)
	})

	sensor := pms5003.NewSensor()
	group.Go(sensor.Start(group.Context()))

	group.Go(func() error {
		for {
			select {
			case reading, ok := <-sensor.Readings():
				if !ok {
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

	err := group.Wait()
	if err != nil {
		log.Fatal("failed to terminate cleanly",
			"err", err)
	}
}

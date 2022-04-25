package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rubiojr/go-enviroplus/pms5003"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stianeikeland/go-rpio/v4"
	"github.com/syncromatics/go-kit/v2/cmd"
	"github.com/syncromatics/go-kit/v2/log"
)

const (
	MetricsPort int = 9100
	j8p13           = 21
	j8p15           = 22
)

var (
	pms_received_packets = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "pms_received_packets",
		},
	)

	// https://cdn-shop.adafruit.com/product-files/3686/plantower-pms5003-manual_v2-3.pdf
	pms_particulate_matter_standard = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pms_particulate_matter_standard",
			Help: "Micrograms per cubic meter, standard particle",
		},
		[]string{"microns"},
	)

	// https://cdn-shop.adafruit.com/product-files/3686/plantower-pms5003-manual_v2-3.pdf
	pms_particulate_matter_environmental = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pms_particulate_matter_environmental",
			Help: "micrograms per cubic meter, adjusted for atmospheric environment",
		},
		[]string{"microns"},
	)

	// https://cdn-shop.adafruit.com/product-files/3686/plantower-pms5003-manual_v2-3.pdf
	pms_particle_counts = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pms_particle_counts",
			Help: "Number of particles with diameter beyond given number of microns in 0.1L of air",
		},
		[]string{"microns_lower_bound"},
	)
)

func main() {
	err := rpio.Open()
	if err != nil {
		log.Fatal("failed to initialize GPIO",
			"err", err)
	}
	defer rpio.Close()

	pin27 := rpio.Pin(j8p13)
	pin27.Mode(rpio.Output)

	pin22 := rpio.Pin(j8p15)
	pin22.Mode(rpio.Output)

	dev, err := pms5003.New()
	if err != nil {
		log.Fatal("failed to initialize UART",
			"err", err)
	}

	dev.EnableDebugging()

	group := cmd.NewProcessGroup(context.Background())
	group.Go(func() error {
		http.Handle("/metrics", promhttp.Handler())
		return http.ListenAndServe(fmt.Sprintf(":%d", MetricsPort), nil)
	})
	group.Go(func() error {
		return dev.StartReadingWithContext(group.Context())
	})
	group.Go(func() error {
		for {
			reading := dev.LastValue()
			log.Debug("last reading",
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
			select {
			case <-group.Context().Done():
				return nil
			case <-time.After(1 * time.Second):
			}
		}
	})

	err = group.Wait()
	if err != nil {
		panic(err)
	}
}

package exporter

import (
	"sensor-exporter/pms5003"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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

func setPMSMetrics(reading *pms5003.Reading) {
	pms_received_packets.Inc()
	pms_particulate_matter_standard.WithLabelValues("01.0").Set(float64(reading.Pm10Std))
	pms_particulate_matter_standard.WithLabelValues("02.5").Set(float64(reading.Pm25Std))
	pms_particulate_matter_standard.WithLabelValues("10.0").Set(float64(reading.Pm100Std))
	pms_particulate_matter_environmental.WithLabelValues("01.0").Set(float64(reading.Pm10Env))
	pms_particulate_matter_environmental.WithLabelValues("02.5").Set(float64(reading.Pm25Env))
	pms_particulate_matter_environmental.WithLabelValues("10.0").Set(float64(reading.Pm100Env))
	pms_particle_counts.WithLabelValues("00.3").Set(float64(reading.Particles3um))
	pms_particle_counts.WithLabelValues("00.5").Set(float64(reading.Particles5um))
	pms_particle_counts.WithLabelValues("01.0").Set(float64(reading.Particles10um))
	pms_particle_counts.WithLabelValues("02.5").Set(float64(reading.Particles25um))
	pms_particle_counts.WithLabelValues("05.0").Set(float64(reading.Particles50um))
	pms_particle_counts.WithLabelValues("10.0").Set(float64(reading.Particles100um))
}

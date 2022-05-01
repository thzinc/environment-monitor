package exporter

import (
	"sensor-exporter/sgp30"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	sgp_received_packets = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "sgp_received_packets",
		},
	)
	sgp_h2_ppm = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "sgp_h2_ppm",
			Help: "Concentration of diatomic hydrogen (H2) in parts per million",
		},
	)
	sgp_ethanol_ppm = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "sgp_ethanol_ppm",
			Help: "Concentration of ethanol in parts per million",
		},
	)
	sgp_tvoc_ppb = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "sgp_tvoc_ppb",
			Help: "Concentration of total volatile organic compounds (VOC) in parts per billion",
		},
	)
	sgp_eco2_ppm = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "sgp_eco2_ppm",
			Help: "Concentration of equivalent carbon dioxide (CO2) in parts per million",
		},
	)
)

func setSGPAirQualityMetrics(reading *sgp30.AirQualityReading) {
	sgp_received_packets.Inc()
	// TODO: include validity
	sgp_tvoc_ppb.Set(float64(reading.TotalVOC))
	sgp_eco2_ppm.Set(float64(reading.EquivalentCO2))
}

func setSGPRawMetrics(reading *sgp30.RawReading) {
	sgp_received_packets.Inc()
	sgp_h2_ppm.Set(float64(reading.H2))
	sgp_ethanol_ppm.Set(float64(reading.Ethanol))
}

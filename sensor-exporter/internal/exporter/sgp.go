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
	sgp_seconds_until_acclimated = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "sgp_seconds_until_acclimated",
			Help: "Number of seconds until the sensor is acclimated to its environment and can be considered to produce valid eCO2 and tVOC readings",
		},
	)
	sgp_tvoc_ppb = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sgp_tvoc_ppb",
			Help: "Concentration of total volatile organic compounds (VOC) in parts per billion",
		},
		[]string{"valid"},
	)
	sgp_eco2_ppm = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sgp_eco2_ppm",
			Help: "Concentration of equivalent carbon dioxide (CO2) in parts per million",
		},
		[]string{"valid"},
	)
)

func setSGPAirQualityMetrics(reading *sgp30.AirQualityReading) {
	sgp_received_packets.Inc()

	var label string
	if reading.IsValid {
		label = "valid"
	} else {
		label = "invalid"
	}
	sgp_eco2_ppm.WithLabelValues(label).Set(float64(reading.EquivalentCO2))
	sgp_tvoc_ppb.WithLabelValues(label).Set(float64(reading.TotalVOC))
	sgp_seconds_until_acclimated.Set(reading.DurationUntilValid.Seconds())
}

func setSGPRawMetrics(reading *sgp30.RawReading) {
	sgp_received_packets.Inc()
	sgp_h2_ppm.Set(float64(reading.H2))
	sgp_ethanol_ppm.Set(float64(reading.Ethanol))
}

package exporter

import (
	"sensor-exporter/aht20"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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

func setAHTMetrics(reading *aht20.Reading) {
	aht_received_packets.Inc()
	aht_relative_humidity.Set(float64(reading.Humidity))
	aht_temperature.Set(float64(reading.Temperature))
}

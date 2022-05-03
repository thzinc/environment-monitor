package exporter

import (
	"sensor-exporter/aht20"
	"sensor-exporter/units"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	aht_received_packets = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "aht_received_packets",
		},
	)
	aht_absolute_humidity = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "aht_absolute_humidity",
			Help: "Concentration of humidity in grams per cubic meter",
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

func setAHTMetrics(reading *aht20.Reading, humidity units.GramsPerCubicMeter) {
	aht_received_packets.Inc()
	aht_absolute_humidity.Set(float64(humidity))
	aht_relative_humidity.Set(float64(reading.Humidity))
	aht_temperature.Set(float64(reading.Temperature))
}

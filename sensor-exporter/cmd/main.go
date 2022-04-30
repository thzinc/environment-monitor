package main

import (
	"context"
	"fmt"
	"net/http"
	"sensor-exporter/aht20"
	"sensor-exporter/pms5003"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/syncromatics/go-kit/v2/cmd"
	"github.com/syncromatics/go-kit/v2/log"
)

const (
	DefaultMetricsPort      int           = 9100
	DefaultPMS5003PortName  string        = "/dev/ttyAMA0"
	DefaultAHT20I2CAddr     uint8         = 0x38
	DefaultAHT20I2CBus      int           = 1
	DefaultReconnectTimeout time.Duration = 1 * time.Second
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
	rootCmd = cobra.Command{
		Use:           "plantower-pms5003-exporter",
		Short:         "start collecting readings from the sensor and host a metrics server",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			settings := &Settings{}
			err := viper.Unmarshal(settings)
			if err != nil {
				return errors.Wrap(err, "failed to parse settings")
			}

			group := cmd.NewProcessGroup(context.Background())

			metricServer := http.Server{
				Addr:    fmt.Sprintf(":%d", settings.MetricsPort),
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

			particulateSensor := pms5003.NewSensor(settings.PMSPortName, settings.ReconnectTimeout)
			log.Info("starting particulate sensor",
				"sensor", particulateSensor)
			group.Go(particulateSensor.Start(group.Context()))
			group.Go(func() error {
				for {
					log.Debug("waiting for reading...")
					select {
					case reading, ok := <-particulateSensor.Readings():
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

			tempHumiditySensor := aht20.NewSensor(settings.AHT20I2CAddr, settings.AHT20I2CBus, settings.ReconnectTimeout)
			log.Info("starting temperature and humidity sensor",
				"sensor", tempHumiditySensor)
			group.Go(tempHumiditySensor.Start(group.Context()))
			group.Go(func() error {
				for {
					log.Debug("waiting for reading...")
					select {
					case reading, ok := <-tempHumiditySensor.Readings():
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

			return group.Wait()
		},
	}
)

// Settings defines the configured settings for the exporter
type Settings struct {
	MetricsPort      int           `mapstructure:"metrics-port"`
	ReconnectTimeout time.Duration `mapstructure:"reconnect-timeout"`
	PMSPortName      string        `mapstructure:"pms5003-port"`
	AHT20I2CAddr     uint8         `mapstructure:"aht20-i2c-addr"`
	AHT20I2CBus      int           `mapstructure:"aht20-i2c-bus"`
}

func init() {
	rootCmd.Flags().Int("metrics-port", DefaultMetricsPort, "Port on which to host Prometheus metrics")
	rootCmd.Flags().Duration("reconnect-timeout", DefaultReconnectTimeout, "Duration to wait before attempting to reconnect to the sensor after a failure")
	rootCmd.Flags().String("pms5003-port", DefaultPMS5003PortName, "Path or name of block device through which to read from the Plantower PMS5003 sensor")
	rootCmd.Flags().Uint8("aht20-i2c-addr", DefaultAHT20I2CAddr, "I2C address of the Asair AHT20 sensor")
	rootCmd.Flags().Int("aht20-i2c-bus", DefaultAHT20I2CBus, "I2C bus to which the Asair ATH20 sensor is attached")

	viper.SetEnvPrefix("EXPORTER")
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv()
	viper.BindPFlags(rootCmd.Flags())
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		log.Fatal("failed to terminate cleanly",
			"err", err)
	}
}

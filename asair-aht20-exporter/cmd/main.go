package main

import (
	"asair-aht20-exporter/aht20"
	"context"
	"fmt"
	"net/http"
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
	DefaultI2CAddr          uint8         = 0x38
	DefaultI2CBus           int           = 1
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
	rootCmd = cobra.Command{
		Use:           "asair-aht20-exporter",
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

			sensor := aht20.NewSensor(settings.I2CAddr, settings.I2CBus, settings.ReconnectTimeout)
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

			return group.Wait()
		},
	}
)

// Settings defines the configured settings for the exporter
type Settings struct {
	MetricsPort      int           `mapstructure:"metrics-port"`
	I2CAddr          uint8         `mapstructure:"i2c-addr"`
	I2CBus           int           `mapstructure:"i2c-bus"`
	ReconnectTimeout time.Duration `mapstructure:"reconnect-timeout"`
}

func init() {
	rootCmd.Flags().Int("metrics-port", DefaultMetricsPort, "Port on which to host Prometheus metrics")
	rootCmd.Flags().Uint8("i2c-addr", DefaultI2CAddr, "I2C address of the Asair AHT20 sensor")
	rootCmd.Flags().Int("i2c-bus", DefaultI2CBus, "I2C bus to which the sensor is attached")
	rootCmd.Flags().Duration("reconnect-timeout", DefaultReconnectTimeout, "Duration to wait before attempting to reconnect to the sensor after a failure")

	viper.SetEnvPrefix("AHT20")
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

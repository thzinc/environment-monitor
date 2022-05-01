package exporter

import (
	"context"
	"fmt"
	"net/http"
	"sensor-exporter/aht20"
	"sensor-exporter/pms5003"
	"sensor-exporter/sgp30"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/pflag"
	"github.com/syncromatics/go-kit/v2/cmd"
	"github.com/syncromatics/go-kit/v2/log"
)

// Settings defines the configured settings for the exporter
type Settings struct {
	MetricsPort      int           `mapstructure:"metrics-port"`
	ReconnectTimeout time.Duration `mapstructure:"reconnect-timeout"`
	PMSPortName      string        `mapstructure:"pms5003-port"`
	AHT20I2CAddr     uint8         `mapstructure:"aht20-i2c-addr"`
	AHT20I2CBus      int           `mapstructure:"aht20-i2c-bus"`
	SGP30I2CAddr     uint8         `mapstructure:"sgp30-i2c-addr"`
	SGP30I2CBus      int           `mapstructure:"sgp30-i2c-bus"`
}

const (
	DefaultMetricsPort      int           = 9100
	DefaultReconnectTimeout time.Duration = 1 * time.Second
	DefaultPMS5003PortName  string        = "/dev/ttyAMA0"
	DefaultAHT20I2CAddr     uint8         = 0x38
	DefaultAHT20I2CBus      int           = 1
	DefaultSGP30I2CAddr     uint8         = 0x58
	DefaultSGP30I2CBus      int           = 1
)

func ConfigureFlags(flags *pflag.FlagSet) {
	flags.Int("metrics-port", DefaultMetricsPort, "Port on which to host Prometheus metrics")
	flags.Duration("reconnect-timeout", DefaultReconnectTimeout, "Duration to wait before attempting to reconnect to the sensor after a failure")
	flags.String("pms5003-port", DefaultPMS5003PortName, "Path or name of block device through which to read from the Plantower PMS5003 sensor")
	flags.Uint8("aht20-i2c-addr", DefaultAHT20I2CAddr, "I2C address of the Asair AHT20 sensor")
	flags.Int("aht20-i2c-bus", DefaultAHT20I2CBus, "I2C bus to which the Asair AHT20 sensor is attached")
	flags.Uint8("sgp30-i2c-addr", DefaultSGP30I2CAddr, "I2C address of the Sensiron SGP30 sensor")
	flags.Int("sgp30-i2c-bus", DefaultSGP30I2CBus, "I2C bus to which the Sensiron SGP30 sensor is attached")
}

func Execute(settings *Settings) error {
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
	group.Go(particulateSensor.Start(group.Context()))

	tempHumiditySensor := aht20.NewSensor(settings.AHT20I2CAddr, settings.AHT20I2CBus, settings.ReconnectTimeout)
	group.Go(tempHumiditySensor.Start(group.Context()))

	gasSensor := sgp30.NewSensor(settings.SGP30I2CAddr, settings.SGP30I2CBus, settings.ReconnectTimeout)
	group.Go(gasSensor.Start(group.Context()))

	group.Go(func() error {
		for {
			select {
			case reading, ok := <-particulateSensor.Readings():
				if !ok {
					log.Debug("particulate sensor readings channel closed")
					return nil
				}

				setPMSMetrics(reading)
			case reading, ok := <-tempHumiditySensor.Readings():
				if !ok {
					log.Debug("temperature and humidity sensor readings channel closed")
					return nil
				}

				setAHTMetrics(reading)
			case reading, ok := <-gasSensor.AirQualityReadings():
				if !ok {
					log.Debug("gas sensor air quality readings channel closed")
					return nil
				}

				log.Debug("received gas sensor air quality reading",
					"reading", reading)

				setSGPAirQualityMetrics(reading)
			case reading, ok := <-gasSensor.RawReadings():
				if !ok {
					log.Debug("gas sensor raw readings channel closed")
					return nil
				}

				log.Debug("received gas sensor raw reading",
					"reading", reading)

				setSGPRawMetrics(reading)
			case <-group.Context().Done():
				return nil
			}
		}
	})

	return group.Wait()
}

package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sensor-exporter/aht20"
	"sensor-exporter/pms5003"
	"sensor-exporter/sgp30"
	"sensor-exporter/units"
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
	BaselineFile     string        `mapstructure:"baseline-file"`
}

const (
	DefaultMetricsPort      int           = 9100
	DefaultReconnectTimeout time.Duration = 1 * time.Second
	DefaultPMS5003PortName  string        = "/dev/ttyAMA0"
	DefaultAHT20I2CAddr     uint8         = 0x38
	DefaultAHT20I2CBus      int           = 1
	DefaultSGP30I2CAddr     uint8         = 0x58
	DefaultSGP30I2CBus      int           = 1
	DefaultBaselineFile     string        = "/var/lib/sensor-exporter/baseline.json"
)

func ConfigureFlags(flags *pflag.FlagSet) {
	flags.Int("metrics-port", DefaultMetricsPort, "Port on which to host Prometheus metrics")
	flags.Duration("reconnect-timeout", DefaultReconnectTimeout, "Duration to wait before attempting to reconnect to the sensor after a failure")
	flags.String("pms5003-port", DefaultPMS5003PortName, "Path or name of block device through which to read from the Plantower PMS5003 sensor")
	flags.Uint8("aht20-i2c-addr", DefaultAHT20I2CAddr, "I2C address of the Asair AHT20 sensor")
	flags.Int("aht20-i2c-bus", DefaultAHT20I2CBus, "I2C bus to which the Asair AHT20 sensor is attached")
	flags.Uint8("sgp30-i2c-addr", DefaultSGP30I2CAddr, "I2C address of the Sensiron SGP30 sensor")
	flags.Int("sgp30-i2c-bus", DefaultSGP30I2CBus, "I2C bus to which the Sensiron SGP30 sensor is attached")
	flags.String("baseline-file", DefaultBaselineFile, "File to store JSON-encoded sensor baseline data to")
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

	initialBaseline := tryReadBaseline(settings.BaselineFile)
	gasSensor := sgp30.NewSensor(settings.SGP30I2CAddr, settings.SGP30I2CBus, settings.ReconnectTimeout, initialBaseline)
	group.Go(gasSensor.Start(group.Context()))

	group.Go(func() error {
		setHumidityAfter := time.Time{}
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

				humidity := units.AbsoluteHumidity(reading.Temperature, reading.Humidity)
				setAHTMetrics(reading, humidity)

				now := time.Now()
				if now.After(setHumidityAfter) {
					setHumidityAfter = now.Add(10 * time.Second)

					log.Debug("setting humidity on gas sensor",
						"humidity", humidity,
						"reading", reading)
					gasSensor.SetHumidity(group.Context(), humidity)
				}
			case reading, ok := <-gasSensor.AirQualityReadings():
				if !ok {
					log.Debug("gas sensor air quality readings channel closed")
					return nil
				}

				setSGPAirQualityMetrics(reading)
			case reading, ok := <-gasSensor.RawReadings():
				if !ok {
					log.Debug("gas sensor raw readings channel closed")
					return nil
				}

				setSGPRawMetrics(reading)
			case baseline, ok := <-gasSensor.BaselineReadings():
				if !ok {
					log.Debug("gas sensor baseline readings channel closed")
					return nil
				}

				tryWriteBaseline(settings.BaselineFile, baseline)
			case <-group.Context().Done():
				return nil
			}
		}
	})

	return group.Wait()
}

func tryReadBaseline(path string) *sgp30.BaselineReading {
	var initialBaseline *sgp30.BaselineReading
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		log.Error("failed to read baseline file; sensor will require acclimation",
			"err", err,
			"path", path)
		return nil
	}

	err = json.Unmarshal(bytes, &initialBaseline)
	if err != nil {
		log.Error("failed to unmarshal baseline file; sensor will require acclimation",
			"err", err,
			"path", path,
			"bytes", bytes)
		return nil
	}

	log.Info("initializing sensor with stored baseline",
		"path", path,
		"initialBaseline", initialBaseline)
	return initialBaseline
}

func tryWriteBaseline(path string, baseline *sgp30.BaselineReading) {
	file, err := json.MarshalIndent(baseline, "", "\t")
	if err != nil {
		log.Error("failed to marshall baseline",
			"err", err,
			"baseline", baseline)
		return
	}

	err = ioutil.WriteFile(path, file, 0644)
	if err != nil {
		log.Error("failed to write baseline file",
			"err", err,
			"file", file,
			"baseline", baseline,
			"path", path)
		return
	}

	log.Info("stored new baseline",
		"path", path,
		"baseline", baseline)
}

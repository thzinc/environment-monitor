package exporter

import (
	"context"
	"fmt"
	"net/http"
	"sensor-exporter/aht20"
	"sensor-exporter/pms5003"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	tempHumiditySensor := aht20.NewSensor(settings.AHT20I2CAddr, settings.AHT20I2CBus, settings.ReconnectTimeout)
	group.Go(particulateSensor.Start(group.Context()))
	group.Go(tempHumiditySensor.Start(group.Context()))
	group.Go(func() error {
		for {
			log.Debug("waiting for readings...")
			select {
			case reading, ok := <-particulateSensor.Readings():
				if !ok {
					log.Debug("particulate sensor readings channel closed")
					return nil
				}

				log.Debug("received particulate sensor reading",
					"reading", reading)

				setPMSMetrics(reading)
			case reading, ok := <-tempHumiditySensor.Readings():
				if !ok {
					log.Debug("temperature and humidity sensor readings channel closed")
					return nil
				}

				log.Debug("received temperature and humidity sensor reading",
					"reading", reading)

				setAHTMetrics(reading)
			case <-group.Context().Done():
				return nil
			}
		}
	})

	return group.Wait()
}

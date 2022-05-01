package main

import (
	"sensor-exporter/internal/exporter"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	rootCmd = cobra.Command{
		Use:           "sensor-exporter",
		Short:         "start collecting readings from sensors and host a metrics server",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			settings := &exporter.Settings{}
			err := viper.Unmarshal(settings)
			if err != nil {
				return errors.Wrap(err, "failed to parse settings")
			}
			log.Info("using settings",
				"settings", settings)

			return exporter.Execute(settings)
		},
	}
)

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

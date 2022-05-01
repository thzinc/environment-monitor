package main

import (
	"sensor-exporter/internal/exporter"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/syncromatics/go-kit/v2/log"
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
	exporter.ConfigureFlags(rootCmd.Flags())

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

package aht20

import (
	"context"
	"io"
	"sensor-exporter/units"
	"time"

	"github.com/d2r2/go-i2c"
	"github.com/d2r2/go-logger"
	"github.com/pkg/errors"
	"github.com/syncromatics/go-kit/v2/log"
	"golang.org/x/sync/errgroup"
)

const (
	wakeUpTimeout time.Duration = 20 * time.Millisecond
	statusTimeout time.Duration = 10 * time.Millisecond
)

// Reading represents the transformed signal from the AHT20 sensor
type Reading struct {
	Humidity    units.RelativeHumidity
	Temperature units.Celsius
}

type Sensor struct {
	i2cAddr          uint8
	i2cBus           int
	readings         chan *Reading
	reconnectTimeout time.Duration
}

func init() {
	logger.ChangePackageLogLevel("i2c", logger.InfoLevel)
}

func NewSensor(
	i2cAddr uint8,
	i2cBus int,
	reconnectTimeout time.Duration,
) *Sensor {
	readings := make(chan *Reading)
	return &Sensor{
		i2cAddr,
		i2cBus,
		readings,
		reconnectTimeout,
	}
}

func (s *Sensor) Readings() <-chan *Reading {
	return s.readings
}

func (s *Sensor) Start(ctx context.Context) func() error {
	return func() error {
		defer close(s.readings)

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(wakeUpTimeout):
			}

			i2c, err := i2c.NewI2C(s.i2cAddr, s.i2cBus)
			if err != nil {
				return errors.Wrapf(err, "failed to open I2C address %v on bus %v", s.i2cAddr, s.i2cBus)
			}

			group, innerCtx := errgroup.WithContext(ctx)
			group.Go(func() error {
				err = reset(innerCtx, i2c)
				if err != nil {
					return errors.Wrap(err, "failed to reset sensor")
				}

				err = calibrate(innerCtx, i2c)
				if err != nil {
					return errors.Wrap(err, "failed to calibrate sensor")
				}

				for {
					reading, err := trigger(innerCtx, i2c)
					if err == io.EOF {
						return nil
					}
					if err != nil {
						return errors.Wrap(err, "failed to trigger reading")
					}

					select {
					case s.readings <- reading:
					case <-innerCtx.Done():
						return nil
					}
				}
			})
			group.Go(func() error {
				<-innerCtx.Done()
				i2c.Close()
				return nil
			})

			err = group.Wait()
			log.Info("disconnected from sensor; waiting to reconnect",
				"err", err,
				"reconnectTimeout", s.reconnectTimeout)

			select {
			case <-ctx.Done():
				return nil
			case <-time.After(s.reconnectTimeout):
				log.Info("reconnecting")
			}
		}
	}
}

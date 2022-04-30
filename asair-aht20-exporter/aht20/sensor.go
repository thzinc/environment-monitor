package aht20

import (
	"context"
	"io"
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

type RelativeHumidity float64
type Celsius float64

// Reading represents the transformed signal from the AHT20 sensor
type Reading struct {
	Humidity    RelativeHumidity
	Temperature Celsius
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

func reset(ctx context.Context, i2c *i2c.I2C) error {
	const cmd_reset byte = 0xBA
	_, err := i2c.WriteBytes([]byte{cmd_reset})
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
	case <-time.After(wakeUpTimeout):
	}

	return nil
}

type statusResponse struct {
	IsCalibrated bool
	IsBusy       bool
}

func status(i2c *i2c.I2C) (*statusResponse, error) {
	buf := make([]byte, 1)
	_, err := i2c.ReadBytes(buf)
	if err != nil {
		return nil, err
	}

	const calibratedMask byte = 0b00001000
	const busyMask byte = 0b10000000

	b := buf[0]
	response := &statusResponse{
		IsCalibrated: b&calibratedMask > 0,
		IsBusy:       b&busyMask > 0,
	}

	return response, nil
}

func calibrate(ctx context.Context, i2c *i2c.I2C) error {
	const cmd_calibrate byte = 0xE1
	_, err := i2c.WriteBytes([]byte{cmd_calibrate, 0x08, 0x00})
	if err != nil {
		return err
	}

	for {
		status, err := status(i2c)
		if err != nil {
			return errors.Wrap(err, "failed to read status")
		}

		if status.IsBusy {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(statusTimeout):
			}
			continue
		}

		if !status.IsCalibrated {
			return errors.New("failed to calibrate sensor")
		}

		return nil
	}
}

func trigger(ctx context.Context, i2c *i2c.I2C) (*Reading, error) {
	const cmd_trigger byte = 0xAC
	_, err := i2c.WriteBytes([]byte{cmd_trigger, 0x33, 0x00})
	if err != nil {
		return nil, err
	}

	for {
		status, err := status(i2c)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read status")
		}

		if status.IsBusy {
			select {
			case <-ctx.Done():
				return nil, io.EOF
			case <-time.After(statusTimeout):
			}
			continue
		}

		buf := make([]byte, 6)
		_, err = i2c.ReadBytes(buf)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read reading")
		}

		/*
		 * buf index 0       1       2       3       4       5
		 *           |-------|-------|-------|-------|-------|-------
		 * category  SSSSSSSSHHHHHHHHHHHHHHHHHHHHTTTTTTTTTTTTTTTTTTTT
		 *
		 * Categories:
		 * S: State (8 bits)
		 * H: Humidity (20 bits)
		 * T: Temperature (20 bits)
		 */

		var rawHumidityReading uint32
		rawHumidityReading = uint32(buf[1])<<12 | uint32(buf[2])<<4 | uint32(buf[3])>>4
		humidity := RelativeHumidity(rawHumidityReading) / 0x100000

		var rawTemperatureReading uint32
		rawTemperatureReading = uint32((buf[3]&0xF))<<16 | uint32(buf[4])<<8 | uint32(buf[5])
		temperature := ((Celsius(rawTemperatureReading) * 200.0) / 0x100000) - 50

		reading := &Reading{
			humidity,
			temperature,
		}
		return reading, nil
	}
}

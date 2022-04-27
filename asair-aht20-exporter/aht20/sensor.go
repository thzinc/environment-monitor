package aht20

import (
	"context"

	"github.com/d2r2/go-i2c"
	"github.com/pkg/errors"
	"github.com/syncromatics/go-kit/v2/log"
	"golang.org/x/sync/errgroup"
)

const (
	i2cAddr uint8 = 0x38
	i2cBus  int   = 1

	cmd_reset     byte = 0xBA
	cmd_calibrate byte = 0xE1
	cmd_trigger   byte = 0xAC

	status_calibrated byte = 0x08
	status_busy       byte = 80
)

type RelativeHumidity float64
type Celsius float64

// Reading represents the data portion of the PMS5003 transport protocol in Active Mode
type Reading struct {
	Humidity    RelativeHumidity
	Temperature Celsius
}

type Sensor struct {
	readings chan *Reading
}

func NewSensor() *Sensor {
	readings := make(chan *Reading)
	return &Sensor{
		readings: readings,
	}
}

func (s *Sensor) Readings() <-chan *Reading {
	return s.readings
}

func (s *Sensor) Start(ctx context.Context) func() error {
	return func() error {
		defer close(s.readings)

		for {
			i2c, err := i2c.NewI2C(i2cAddr, i2cBus)
			if err != nil {
				return errors.Wrapf(err, "failed to open I2C address %v on bus %v", i2cAddr, i2cBus)
			}

			readStatusByte := func() (byte, error) {
				buf := make([]byte, 1)
				_, err := i2c.ReadBytes(buf)
				if err != nil {
					return 0, err
				}
				return buf[0], nil
			}

			group, innerCtx := errgroup.WithContext(ctx)
			group.Go(func() error {
				_, err = i2c.WriteBytes([]byte{cmd_reset})
				if err != nil {
					return errors.Wrap(err, "failed to reset sensor")
				}

				_, err = i2c.WriteBytes([]byte{cmd_calibrate, 0x08, 0x00})
				if err != nil {
					return errors.Wrap(err, "failed to calibrate sensor")
				}

				status, err := readStatusByte()
				if err != nil {
					return errors.Wrap(err, "failed to read status")
				}
				if status != status_calibrated {
					return errors.Errorf("failed with %x after attempting to calibrate sensor", status)
				}

				for {
					_, err = i2c.WriteBytes([]byte{cmd_trigger, 0x33, 0x00})
					if err != nil {
						return errors.Wrap(err, "failed to trigger reading")
					}

					for {
						status, err := readStatusByte()
						if err != nil {
							return errors.Wrap(err, "failed to read status")
						}
						if status == status_busy {
							continue
						}
						break
					}

					buf := make([]byte, 6)
					_, err := i2c.ReadBytes(buf)
					if err != nil {
						return errors.Wrap(err, "failed to read reading")
					}

					var rawHumidityReading uint32
					rawHumidityReading = uint32(buf[1])<<12 | uint32(buf[2])<<4 | uint32(buf[3])>>4
					humidity := (RelativeHumidity(rawHumidityReading) * 100) / 0x100000

					var rawTemperatureReading uint32
					rawTemperatureReading = uint32((buf[3]&0xF))<<16 | uint32(buf[4])<<8 | uint32(buf[5])
					temperature := ((Celsius(rawTemperatureReading) * 200.0) / 0x100000) - 50

					reading := &Reading{
						humidity,
						temperature,
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
			log.Debug("failed to stay connected",
				"err", err)
		}
	}
}

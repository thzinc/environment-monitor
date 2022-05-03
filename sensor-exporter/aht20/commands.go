/*
 * This implementation was based on the Adafruit AHT20 CircuitPython implementation: https://github.com/adafruit/Adafruit_CircuitPython_AHTx0
 * I've since learned that this implementation uses the AHT10 set of commands, which the AHT20 appears to support
 */
package aht20

import (
	"context"
	"io"
	"sensor-exporter/units"
	"time"

	"github.com/d2r2/go-i2c"
	"github.com/pkg/errors"
)

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
		humidity := units.RelativeHumidity(rawHumidityReading) / 0x100000

		var rawTemperatureReading uint32
		rawTemperatureReading = uint32((buf[3]&0xF))<<16 | uint32(buf[4])<<8 | uint32(buf[5])
		temperature := ((units.Celsius(rawTemperatureReading) * 200.0) / 0x100000) - 50

		reading := &Reading{
			humidity,
			temperature,
		}
		return reading, nil
	}
}

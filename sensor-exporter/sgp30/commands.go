package sgp30

import (
	"context"
	"io"
	"sensor-exporter/units"
	"time"

	"github.com/d2r2/go-i2c"
	"github.com/pkg/errors"
	"github.com/sigurn/crc8"
)

func getSerialID(ctx context.Context, i2c *i2c.I2C) ([]uint16, error) {
	_, err := i2c.WriteBytes([]byte{0x36, 0x82})
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, io.EOF
	case <-time.After(10 * time.Millisecond):
	}

	serial, err := readWords(i2c, 3)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read serial")
	}

	return serial, nil
}

func getFeatureSetVersion(i2c *i2c.I2C) (uint16, error) {
	_, err := i2c.WriteBytes([]byte{0x20, 0x2F})
	if err != nil {
		return 0, err
	}

	data, err := readWords(i2c, 1)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read feature set version")
	}

	return data[0], nil
}

var (
	supportedFeatureSets = map[uint16]bool{
		0x0020: true,
		0x0022: true,
	}
)

func isSupportedFeatureSetVersion(i2c *i2c.I2C) (bool, uint16, error) {
	featureSet, err := getFeatureSetVersion(i2c)
	if err != nil {
		return false, 0, err
	}

	_, exists := supportedFeatureSets[featureSet]
	return exists, featureSet, nil
}

func initAirQuality(ctx context.Context, i2c *i2c.I2C) error {
	_, err := i2c.WriteBytes([]byte{0x20, 0x03})
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
	case <-time.After(10 * time.Millisecond):
	}

	return nil
}
func measureAirQuality(ctx context.Context, i2c *i2c.I2C) ([]uint16, error) {
	_, err := i2c.WriteBytes([]byte{0x20, 0x08})
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, io.EOF
	case <-time.After(12 * time.Millisecond):
	}

	data, err := readWords(i2c, 2)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read air quality")
	}

	return data, nil
}

func measureRawSignals(ctx context.Context, i2c *i2c.I2C) ([]uint16, error) {
	_, err := i2c.WriteBytes([]byte{0x20, 0x50})
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, io.EOF
	case <-time.After(25 * time.Millisecond):
	}

	data, err := readWords(i2c, 2)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read raw signals")
	}

	return data, nil
}

func getBaseline(ctx context.Context, i2c *i2c.I2C) ([]uint16, error) {
	_, err := i2c.WriteBytes([]byte{0x20, 0x15})
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, io.EOF
	case <-time.After(10 * time.Millisecond):
	}

	data, err := readWords(i2c, 2)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read raw signals")
	}

	return data, nil
}

func setBaseline(ctx context.Context, i2c *i2c.I2C, eCO2, tVOC uint16) error {
	eCO2data := []byte{byte(eCO2 >> 8), byte(eCO2)}
	eCO2crc := crc8.Checksum(eCO2data, checksumTable)
	tVOCdata := []byte{byte(tVOC >> 8), byte(tVOC)}
	tVOCcrc := crc8.Checksum(tVOCdata, checksumTable)

	command := []byte{0x20, 0x1E}
	_, err := i2c.WriteBytes(append(command, eCO2data[0], eCO2data[1], eCO2crc, tVOCdata[0], tVOCdata[1], tVOCcrc))
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return io.EOF
	case <-time.After(10 * time.Millisecond):
	}
	return nil
}

func setHumidity(ctx context.Context, i2c *i2c.I2C, humidity units.GramsPerCubicMeter) error {
	fixedPointValue := uint16(humidity * 256)
	humidityData := []byte{byte(fixedPointValue >> 8), byte(fixedPointValue)}
	humidityCRC := crc8.Checksum(humidityData, checksumTable)

	command := []byte{0x20, 0x61}
	_, err := i2c.WriteBytes(append(command, humidityData[0], humidityData[1], humidityCRC))
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return io.EOF
	case <-time.After(10 * time.Millisecond):
	}
	return nil
}

var (
	checksumTable = crc8.MakeTable(crc8.Params{
		Poly:   0x31,
		Init:   0xFF,
		RefIn:  false,
		RefOut: false,
		XorOut: 0x00,
		Check:  0x00,
		Name:   "CRC-8/Sensiron",
	})
)

func readWords(i2c *i2c.I2C, words int) ([]uint16, error) {
	const (
		wordLength = 2
		crcLength  = 1
	)

	buf := make([]byte, words*(wordLength+crcLength))
	_, err := i2c.ReadBytes(buf)
	if err != nil {
		return nil, err
	}

	data := []uint16{}
	for idx := 0; idx < len(buf); idx += wordLength + crcLength {
		wordBytes := buf[idx : idx+2]
		expectedCrc := buf[idx+2]
		actualCrc := crc8.Checksum(wordBytes, checksumTable)
		if actualCrc != expectedCrc {
			return nil, errors.Errorf("failed to validate crc for %v (expected %v but got %v)", wordBytes, expectedCrc, actualCrc)
		}

		word := uint16(wordBytes[0])<<8 | uint16(wordBytes[1])
		data = append(data, word)
	}
	return data, nil
}

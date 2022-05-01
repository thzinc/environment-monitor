package sgp30

import (
	"context"
	"io"
	"time"

	"github.com/d2r2/go-i2c"
	"github.com/d2r2/go-logger"
	"github.com/pkg/errors"
	"github.com/sigurn/crc8"
	"github.com/syncromatics/go-kit/v2/log"
	"golang.org/x/sync/errgroup"
)

type PartsPerBillion uint16
type PartsPerMillion uint16

// AirQualityReading represents the transformed air quality signal from the SGP30 sensor
type AirQualityReading struct {
	// Indicates whether the reading can be considered valid depending on the initialization of the sensor and its running time
	IsValid bool
	// Total volatile organic compound (VOC) concentration in parts per billion
	TotalVOC PartsPerBillion
	// Equivalent carbon dioxide (CO2) concentration in parts per million
	EquivalentCO2 PartsPerMillion
}

// RawReading represents the transformed raw signal from the SGP30 sensor
type RawReading struct {
	// Concentration of diatomic hydrogen (H2) in parts per million
	H2 PartsPerMillion
	// Concentration of ethanol in parts per million
	Ethanol PartsPerMillion
}

type Sensor struct {
	i2cAddr            uint8
	i2cBus             int
	airQualityReadings chan *AirQualityReading
	rawReadings        chan *RawReading
	reconnectTimeout   time.Duration
}

func init() {
	logger.ChangePackageLogLevel("i2c", logger.InfoLevel)
}

func NewSensor(
	i2cAddr uint8,
	i2cBus int,
	reconnectTimeout time.Duration,
) *Sensor {
	airQualityReadings := make(chan *AirQualityReading)
	rawReadings := make(chan *RawReading)
	return &Sensor{
		i2cAddr,
		i2cBus,
		airQualityReadings,
		rawReadings,
		reconnectTimeout,
	}
}

func (s *Sensor) AirQualityReadings() <-chan *AirQualityReading {
	return s.airQualityReadings
}

func (s *Sensor) RawReadings() <-chan *RawReading {
	return s.rawReadings
}

func (s *Sensor) Start(ctx context.Context) func() error {
	return func() error {
		defer close(s.airQualityReadings)
		defer close(s.rawReadings)

		for {
			i2c, err := i2c.NewI2C(s.i2cAddr, s.i2cBus)
			if err != nil {
				return errors.Wrapf(err, "failed to open I2C address %v on bus %v", s.i2cAddr, s.i2cBus)
			}

			group, innerCtx := errgroup.WithContext(ctx)
			group.Go(func() error {
				serial, err := getSerialID(innerCtx, i2c)
				if err != nil {
					return errors.Wrap(err, "failed to read serial")
				}

				isSupported, featureSet, err := isSupportedFeatureSetVersion(i2c)
				if err != nil {
					return errors.Wrap(err, "failed to read feature set")
				}
				if !isSupported {
					return errors.Errorf("failed to support feature set %X", featureSet)
				}

				log.Debug("read serial",
					"serial", serial,
					"featureSet", featureSet)

				// TODO: init

				for {
					airQualityReadings, err := measureAirQuality(innerCtx, i2c)
					if err != nil {
						return errors.Wrap(err, "failed to read air quality")
					}
					airQualityReading := &AirQualityReading{
						IsValid:       false, // HACK
						TotalVOC:      PartsPerBillion(airQualityReadings[1]),
						EquivalentCO2: PartsPerMillion(airQualityReadings[0]),
					}
					select {
					case <-innerCtx.Done():
						return nil
					case s.airQualityReadings <- airQualityReading:
					}

					rawReadings, err := measureRawSignals(innerCtx, i2c)
					if err != nil {
						return errors.Wrap(err, "failed to read raw signals")
					}
					rawReading := &RawReading{
						H2:      PartsPerMillion(rawReadings[0]),
						Ethanol: PartsPerMillion(rawReadings[1]),
					}
					select {
					case <-innerCtx.Done():
						return nil
					case s.rawReadings <- rawReading:
					}

					// TODO: get baseline
					// TODO: set baseline
					// TODO: set humidity
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

func getSerialID(ctx context.Context, i2c *i2c.I2C) ([]uint16, error) {
	cmd_serial := []byte{0x36, 0x82}
	_, err := i2c.WriteBytes(cmd_serial)
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

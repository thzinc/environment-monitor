package sgp30

import (
	"context"
	"sensor-exporter/units"
	"time"

	"github.com/d2r2/go-i2c"
	"github.com/d2r2/go-logger"
	"github.com/pkg/errors"
	"github.com/syncromatics/go-kit/v2/log"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
)

type PartsPerBillion uint16
type PartsPerMillion uint16

type requestAirQualityReading struct{}

// AirQualityReading represents the transformed air quality signal from the SGP30 sensor
type AirQualityReading struct {
	// Indicates whether the reading can be considered valid depending on the initialization of the sensor and its running time
	IsValid bool
	// Remaining duration until the air quality readings can be considered valid
	DurationUntilValid time.Duration
	// Total volatile organic compound (VOC) concentration in parts per billion
	TotalVOC PartsPerBillion
	// Equivalent carbon dioxide (CO2) concentration in parts per million
	EquivalentCO2 PartsPerMillion
}

type requestBaselineReading struct {
	serial                       []uint16
	sensorReadingsNotValidBefore time.Time
}

type BaselineReading struct {
	// Serial number of the sensor
	Serial []uint16
	// Time at which the sensor is expected to have acclimated to the environment. Readings before this time may be suspect.
	SensorReadingsNotValidBefore time.Time
	// Time at which this baseline reading is invalid
	BaselineInvalidAfter time.Time
	// Total volatile organic compound (VOC) concentration in parts per billion
	TotalVOC PartsPerBillion
	// Equivalent carbon dioxide (CO2) concentration in parts per million
	EquivalentCO2 PartsPerMillion
}

type requestRawReading struct{}

// RawReading represents the transformed raw signal from the SGP30 sensor
type RawReading struct {
	// Concentration of diatomic hydrogen (H2) in parts per million
	H2 PartsPerMillion
	// Concentration of ethanol in parts per million
	Ethanol PartsPerMillion
}

type becomeInitialized struct{}

type updateHumidity struct {
	temperature      units.Celsius
	relativeHumidity units.RelativeHumidity
}

type Sensor struct {
	i2cAddr            uint8
	i2cBus             int
	airQualityReadings chan *AirQualityReading
	rawReadings        chan *RawReading
	baselineReadings   chan *BaselineReading
	reconnectTimeout   time.Duration
	commands           chan interface{}
	initialBaseline    *BaselineReading
}

func init() {
	logger.ChangePackageLogLevel("i2c", logger.InfoLevel)
}

func NewSensor(
	i2cAddr uint8,
	i2cBus int,
	reconnectTimeout time.Duration,
	initialBaseline *BaselineReading,
) *Sensor {
	airQualityReadings := make(chan *AirQualityReading)
	rawReadings := make(chan *RawReading)
	baselineReadings := make(chan *BaselineReading)
	commands := make(chan interface{})
	return &Sensor{
		i2cAddr,
		i2cBus,
		airQualityReadings,
		rawReadings,
		baselineReadings,
		reconnectTimeout,
		commands,
		initialBaseline,
	}
}

func (s *Sensor) AirQualityReadings() <-chan *AirQualityReading {
	return s.airQualityReadings
}

func (s *Sensor) RawReadings() <-chan *RawReading {
	return s.rawReadings
}

func (s *Sensor) BaselineReadings() <-chan *BaselineReading {
	return s.baselineReadings
}

func (s *Sensor) SetRelativeHumidity(ctx context.Context, temperature units.Celsius, relativeHumidity units.RelativeHumidity) {
	command := &updateHumidity{
		temperature,
		relativeHumidity,
	}
	go func() {
		select {
		case <-ctx.Done():
		case s.commands <- command:
		}
	}()
}

func (s *Sensor) Start(ctx context.Context) func() error {
	return func() error {
		defer close(s.airQualityReadings)
		defer close(s.rawReadings)
		defer close(s.baselineReadings)
		defer close(s.commands)
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

				err = initAirQuality(innerCtx, i2c)
				if err != nil {
					return errors.Wrap(err, "failed to initialize air quality")
				}

				now := time.Now()
				var sensorReadingsNotValidBefore time.Time
				if s.initialBaseline != nil &&
					slices.Equal(s.initialBaseline.Serial, serial) &&
					now.Before(s.initialBaseline.BaselineInvalidAfter) {
					sensorReadingsNotValidBefore = s.initialBaseline.SensorReadingsNotValidBefore

					err = setBaseline(innerCtx, i2c, uint16(s.initialBaseline.EquivalentCO2), uint16(s.initialBaseline.TotalVOC))
					if err != nil {
						return errors.Wrap(err, "failed to set baseline")
					}
				} else {
					log.Warn("failed to set baseline; sensor will require acclimation",
						"serial", serial,
						"now", now,
						"initialBaseline", s.initialBaseline)
					sensorReadingsNotValidBefore = now.Add(12 * time.Hour)
				}

				group.Go(s.handleCommands(innerCtx, i2c, sensorReadingsNotValidBefore))
				group.Go(s.scheduleRepeatedly(innerCtx, &requestAirQualityReading{}, 1*time.Second))
				group.Go(s.scheduleRepeatedly(innerCtx, &requestRawReading{}, 25*time.Millisecond))
				group.Go(s.scheduleRepeatedly(innerCtx, &requestBaselineReading{
					serial,
					sensorReadingsNotValidBefore,
				}, 1*time.Hour))
				group.Go(s.scheduleOnce(innerCtx, &becomeInitialized{}, 15*time.Second))

				return nil
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

func (s *Sensor) scheduleOnce(ctx context.Context, command interface{}, duration time.Duration) func() error {
	return func() error {
		select {
		case <-ctx.Done():
		case <-time.After(duration):
			select {
			case <-ctx.Done():
			case s.commands <- command:
			}
		}
		return nil
	}
}

func (s *Sensor) scheduleRepeatedly(ctx context.Context, command interface{}, interval time.Duration) func() error {
	return func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(interval):
				select {
				case <-ctx.Done():
					return nil
				case s.commands <- command:
				}
			}
		}
	}
}

func (s *Sensor) handleCommands(innerCtx context.Context, i2c *i2c.I2C, sensorReadingsNotValidBefore time.Time) func() error {
	return func() error {
		isInitialized := false
		for {
			select {
			case <-innerCtx.Done():
				return nil
			case c := <-s.commands:
				switch command := c.(type) {
				case *becomeInitialized:
					isInitialized = true
				case *requestAirQualityReading:
					airQualityReadings, err := measureAirQuality(innerCtx, i2c)
					if err != nil {
						return errors.Wrap(err, "failed to read air quality")
					}

					now := time.Now()
					isSensorAcclimated := now.After(sensorReadingsNotValidBefore)
					isValid := isInitialized && isSensorAcclimated
					var durationUntilValid time.Duration
					if isSensorAcclimated {
						durationUntilValid = time.Duration(0)
					} else {
						durationUntilValid = sensorReadingsNotValidBefore.Sub(now)
					}

					airQualityReading := &AirQualityReading{
						IsValid:            isValid,
						DurationUntilValid: durationUntilValid,
						EquivalentCO2:      PartsPerMillion(airQualityReadings[0]),
						TotalVOC:           PartsPerBillion(airQualityReadings[1]),
					}
					select {
					case <-innerCtx.Done():
						return nil
					case s.airQualityReadings <- airQualityReading:
					}
				case *requestRawReading:
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
				case *requestBaselineReading:
					baseline, err := getBaseline(innerCtx, i2c)
					if err != nil {
						return errors.Wrap(err, "failed to read baseline")
					}
					baselineReading := &BaselineReading{
						Serial:                       command.serial,
						SensorReadingsNotValidBefore: command.sensorReadingsNotValidBefore,
						BaselineInvalidAfter:         time.Now().Add(7 * 24 * time.Hour),
						EquivalentCO2:                PartsPerMillion(baseline[0]),
						TotalVOC:                     PartsPerBillion(baseline[1]),
					}
					s.initialBaseline = baselineReading
					select {
					case <-innerCtx.Done():
						return nil
					case s.baselineReadings <- baselineReading:
					}
				case *updateHumidity:
					err := setRelativeHumidity(innerCtx, i2c, command.temperature, command.relativeHumidity)
					if err != nil {
						return errors.Wrap(err, "failed to set humidity")
					}
				default:
					log.Warn("failed to handle unknown command",
						"command", command)
				}
			}
		}
	}
}

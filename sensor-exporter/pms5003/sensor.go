package pms5003

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"time"

	"github.com/pkg/errors"
	"github.com/syncromatics/go-kit/v2/log"
	"github.com/tarm/serial"
	"golang.org/x/sync/errgroup"
)

const (
	startCharacter1 byte = 0x42
	startCharacter2 byte = 0x4d
)

type StandardParticleConcentration uint16
type EnvironmentalParticleConcentration uint16
type CountPerDeciliter uint16

// Reading represents the data portion of the PMS5003 transport protocol in Active Mode
type Reading struct {
	// Frame length
	Length uint16
	// PM1.0 concentration unit μ g/m3 (CF=1，standard particle)
	Pm10Std StandardParticleConcentration
	// PM2.5 concentration unit μ g/m3 (CF=1，standard particle)
	Pm25Std StandardParticleConcentration
	// PM10 concentration unit μ g/m3 (CF=1，standard particle)
	Pm100Std StandardParticleConcentration
	// PM1.0 concentration unit μ g/m3 (under atmospheric environment)
	Pm10Env EnvironmentalParticleConcentration
	// PM2.5 concentration unit μ g/m3 (under atmospheric environment)
	Pm25Env EnvironmentalParticleConcentration
	// PM10 concentration unit μ g/m3 (under atmospheric environment)
	Pm100Env EnvironmentalParticleConcentration
	// Number of particles with diameter beyond 0.3 um in 0.1L of air.
	Particles3um CountPerDeciliter
	// Number of particles with diameter beyond 0.5 um in 0.1L of air.
	Particles5um CountPerDeciliter
	// Number of particles with diameter beyond 1.0 um in 0.1L of air.
	Particles10um CountPerDeciliter
	// Number of particles with diameter beyond 2.5 um in 0.1L of air.
	Particles25um CountPerDeciliter
	// Number of particles with diameter beyond 5.0 um in 0.1L of air.
	Particles50um CountPerDeciliter
	// Number of particles with diameter beyond 10.0 um in 0.1L of air.
	Particles100um CountPerDeciliter
	// Reserved
	Unused uint16
	// Check code
	Checksum uint16
}

type Sensor struct {
	portName         string
	readings         chan *Reading
	reconnectTimeout time.Duration
}

func NewSensor(portName string, reconnectTimeout time.Duration) *Sensor {
	readings := make(chan *Reading)
	return &Sensor{
		portName,
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
			config := &serial.Config{
				Name:     s.portName,
				Baud:     9600,
				Size:     8,
				Parity:   serial.ParityNone,
				StopBits: serial.Stop1,
			}
			port, err := serial.OpenPort(config)
			if err != nil {
				return errors.Wrapf(err, "failed to open port %v", config.Name)
			}

			reader := bufio.NewReader(port)
			group, innerCtx := errgroup.WithContext(ctx)
			group.Go(func() error {
				for {
					err = seekToRecordStart(innerCtx, reader)
					if err != nil {
						return errors.Wrap(err, "failed to seek to start of record")
					}

					buf := make([]byte, 30)
					_, err := io.ReadFull(reader, buf)
					if err != nil {
						return errors.Wrap(err, "failed to read record")
					}

					rdr := bytes.NewReader(buf)
					reading := &Reading{}
					err = binary.Read(rdr, binary.BigEndian, reading)
					if err != nil {
						return errors.Wrapf(err, "failed to read %v into struct", buf)
					}

					var expectedChecksum uint16 = uint16(startCharacter1) + uint16(startCharacter2)
					for i := 0; i < 28; i++ {
						expectedChecksum += uint16(buf[i])
					}

					if reading.Checksum != expectedChecksum {
						log.Debug("failed to validate checksum",
							"buf", buf,
							"reading", reading,
							"expectedChecksum", expectedChecksum)
						continue
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
				port.Close()
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

func seekToRecordStart(ctx context.Context, reader *bufio.Reader) error {
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return err
		}
		if b != startCharacter1 {
			continue
		}

		b, err = reader.ReadByte()
		if err != nil {
			return err
		}
		if b != startCharacter2 {
			continue
		}

		return nil
	}
}

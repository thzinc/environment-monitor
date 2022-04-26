package pms5003

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
	"github.com/tarm/serial"
	"golang.org/x/sync/errgroup"
)

const (
	startCharacter1 byte = 0x42
	startCharacter2 byte = 0x4d
)

// Reading represents the data portion of the PMS5003 transport protocol in Active Mode
type Reading struct {
	// Frame length
	Length uint16
	// PM1.0 concentration unit μ g/m3 (CF=1，standard particle)
	Pm10Std uint16
	// PM2.5 concentration unit μ g/m3 (CF=1，standard particle)
	Pm25Std uint16
	// PM10 concentration unit μ g/m3 (CF=1，standard particle)
	Pm100Std uint16
	// PM1.0 concentration unit μ g/m3 (under atmospheric environment)
	Pm10Env uint16
	// PM2.5 concentration unit μ g/m3 (under atmospheric environment)
	Pm25Env uint16
	// PM10 concentration unit μ g/m3 (under atmospheric environment)
	Pm100Env uint16
	// Number of particles with diameter beyond 0.3 um in 0.1L of air.
	Particles3um uint16
	// Number of particles with diameter beyond 0.5 um in 0.1L of air.
	Particles5um uint16
	// Number of particles with diameter beyond 1.0 um in 0.1L of air.
	Particles10um uint16
	// Number of particles with diameter beyond 2.5 um in 0.1L of air.
	Particles25um uint16
	// Number of particles with diameter beyond 5.0 um in 0.1L of air.
	Particles50um uint16
	// Number of particles with diameter beyond 10.0 um in 0.1L of air.
	Particles100um uint16
	// Reserved
	Unused uint16
	// Check code
	Checksum uint16
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

		config := &serial.Config{
			Name:     "/dev/ttyAMA0",
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
					continue
					// return errors.Errorf("failed to validate reading checksum %v of %v against expected %v", reading.Checksum, buf, expectedChecksum)
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

		return group.Wait()
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

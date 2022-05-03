package units

import "math"

type RelativeHumidity float64
type Celsius float64
type GramsPerCubicMeter float64

func AbsoluteHumidity(temperature Celsius, relativeHumidity RelativeHumidity) GramsPerCubicMeter {
	// Adapted from https://github.com/skgrange/threadr/blob/fd42380883133fe7a47c479e778afe644a507334/R/absolute_humidity.R
	rh := float64(relativeHumidity) * 100
	t := float64(temperature)
	h := (6.112 * math.Exp((17.67*t)/(t+243.5)) * rh * 2.1674) / (273.15 + t)

	humidity := GramsPerCubicMeter(h)
	return humidity
}

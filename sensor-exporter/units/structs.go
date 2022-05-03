package units

import "math"

type RelativeHumidity float64
type Celsius float64
type GramsPerCubicMeter float64

func AbsoluteHumidity(temperature Celsius, relativeHumidity RelativeHumidity) GramsPerCubicMeter {
	rh := float64(relativeHumidity)
	c := float64(temperature)
	numerator := rh * 6.112 * math.Exp((17.62*c)/(243.12+c))
	denominator := 273.15 + c

	humidity := GramsPerCubicMeter(216.7 * (numerator / denominator))
	return humidity
}

# Environment Monitor

An IoT project to collect local environmental data using:

- Raspberry Pi 3
- Asair AHT20 temperature and humidity sensor
- Sensiron SGP30 gas sensor
- Plantower PMS5003 particulate sensor

Grafana & Prometheus config courtesy of https://github.com/balenalabs-incubator/balena-prometheus-grafana

Better README to come

# Unorganized Notes

- UART: https://www.electronicwings.com/raspberry-pi/raspberry-pi-uart-communication-using-python-and-c
- I2C: https://www.balena.io/docs/learn/develop/hardware/i2c-and-spi/#i2c
  - https://forums.balena.io/t/modprobe-error-when-trying-to-enable-i2c-on-rpi/4669
  - https://www.balena.io/docs/reference/supervisor/docker-compose/#labels
  - I dunno, maybe? https://learn.sparkfun.com/tutorials/qwiic-kit-for-raspberry-pi-hookup-guide/troubleshooting
- Balena hardware stuff: https://www.balena.io/docs/learn/develop/hardware/
- Balena examples for priv mode and labels: https://github.com/balenalabs-incubator/boombeastic/blob/master/docker-compose.yml
- "breathe" https://www.markhansen.co.nz/raspberry-pi-air-quality-sensor/
  - also https://github.com/mhansen/breathe/blob/master/breathe.go
- https://stackoverflow.com/questions/39320025/how-to-stop-http-listenandserve
- UGH: https://www.raspberrypi.com/documentation/computers/config_txt.html#enable_uart
- https://www.balena.io/docs/reference/supervisor/configuration-list/raspberrypi3/
- https://www.raspberrypi.com/documentation/computers/config_txt.html#enable_uart
- https://www.balena.io/docs/reference/OS/advanced/
- Issues with UART on Raspberry Pi 3 specifically: https://forums.balena.io/t/disable-console-over-serial-in-dev-on-rpi3/1412/21
- Helpful
  - https://learn.adafruit.com/adafruit-sgp30-gas-tvoc-eco2-mox-sensor/circuitpython-wiring-test
  - https://github.dev/adafruit/Adafruit_CircuitPython_SGP30/blob/main/adafruit_sgp30.py

On host OS

```bash
mount -o remount,rw /
systemctl mask serial-getty@ttyAMA0.service
systemctl mask serial-getty@serial0.service
systemctl mask serial-getty@serial1.service
reboot
```

## AQI notes

- https://www.airnow.gov/sites/default/files/2020-05/aqi-technical-assistance-document-sept2018.pdf
- https://forum.airnowtech.org/t/aqi-calculations-overview-ozone-pm2-5-and-pm10/168
- https://forums.adafruit.com/viewtopic.php?f=48&t=136528&p=772070&hilit=plantower#p772057
- https://amt.copernicus.org/articles/14/4617/2021/amt-14-4617-2021-discussion.html
- https://www3.epa.gov/region1/airquality/pm-aq-standards.html
- https://www.gwern.net/zeo/CO2

/**
 * Based off of https://github.com/networkupstools/nut/blob/v2.7.4/drivers/tripplite_usb.c
 */

package tripplite

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gotmc/libusb/v2"
	"github.com/rs/zerolog/log"

	golog "log"
)

var (
	PROTOCOL_LOOKUP = map[uint]string{
		0x3003: "SMARTPRO",
	}
)

func int_to_hex(val uint16) string {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, val)
	return hex.EncodeToString(b)
}

func strconv_clean(data []byte) string {
	for i, b := range data {
		if !unicode.IsPrint(rune(b)) {
			data[i] = '.'
		}
	}
	return strings.Trim(string(data), ".")
}

func get_protocol_name(protocol uint) string {
	if v, ok := PROTOCOL_LOOKUP[protocol]; ok {
		return v
	}
	return ""
}

func usbGetStringOrDefault(handle *libusb.DeviceHandle, attr uint8, defaultVal string) string {
	val, err := handle.StringDescriptorASCII(attr)
	if err != nil || len(val) == 0 {
		return defaultVal
	}
	return val
}

type SmartProUPSMonitor struct {
	ctx                   *libusb.Context
	dev                   *libusb.Device
	h                     *libusb.DeviceHandle
	interfaceId           uint16
	endpointAddress       byte
	txTimeout             uint16 // milliseconds
	rxTimeout             uint16 // milliseconds
	maxPacketSize         uint16
	Protocol              uint
	ProtocolName          string
	VendorId              uint16
	ProductId             uint16
	Manufacturer          string
	Product               string
	Serial                string
	streaming             bool
	debugUSB              bool
	resetVoltageResetEver bool
}

func NewSmartProUPSMonitor(vid uint16, pid uint16) (*SmartProUPSMonitor, error) {

	ctx, err := libusb.NewContext()
	if err != nil {
		log.Warn().Err(err).Msg("failed to create context")
		return nil, err
	}

	log.Debug().Uint16("vid", vid).Uint16("pid", pid).Msg("opening device")
	golog.SetOutput(io.Discard)
	dev, h, err := ctx.OpenDeviceWithVendorProduct(vid, pid)
	if err != nil {
		log.Warn().Err(err).Uint16("vid", vid).Uint16("pid", pid).Msg("failed to find device")
		ctx.Close()
		return nil, err
	}

	mon := SmartProUPSMonitor{
		ctx:             ctx,
		dev:             dev,
		h:               h,
		interfaceId:     0,
		endpointAddress: 0x81,
		txTimeout:       5000,
		rxTimeout:       5000,
		maxPacketSize:   0,
		Protocol:        0,
		ProtocolName:    "",
		VendorId:        vid,
		ProductId:       pid,
		Manufacturer:    "",
		Product:         "",
		Serial:          "",
		streaming:       false,
		debugUSB:        false,
	}

	dd, err := dev.DeviceDescriptor()
	if err != nil {
		mon.Close()
		return nil, err
	}

	mon.maxPacketSize = uint16(dd.MaxPacketSize0)
	mon.Manufacturer = strings.TrimSpace(usbGetStringOrDefault(h, dd.ManufacturerIndex, ""))
	mon.Product = strings.TrimSpace(usbGetStringOrDefault(h, dd.ProductIndex, ""))
	mon.Serial = strings.TrimSpace(usbGetStringOrDefault(h, dd.SerialNumberIndex, ""))

	err = mon.Claim()
	if err != nil {
		log.Error().Err(err).Uint16("interface", mon.interfaceId).Msg("unable to claim interface")
		mon.Close()
		return nil, err
	}

	reply, err := mon.SendCommand([]byte{0})
	if err == nil {
		mon.Protocol = (uint(reply[1]) << 8) | uint(reply[2])
		mon.ProtocolName = get_protocol_name(mon.Protocol)
	} else {
		mon.Close()
		return nil, err
	}

	return &mon, nil
}

func (m *SmartProUPSMonitor) setReport(reportId uint16, msg []byte) (int, error) {

	bytes_sent, err := m.h.ControlTransfer(
		0x00+(0x01<<5)+0x01, // requestType
		0x09,                // request
		reportId+(0x03<<8),  // value
		0,                   // HID interface index
		msg,
		len(msg),
		int(m.txTimeout)) // Timeout

	if bytes_sent != len(msg) && err == nil {
		err = errors.New(fmt.Sprint("failed to send report, sent", bytes_sent, "expected", len(msg)))
	}

	return bytes_sent, err
}

func (m *SmartProUPSMonitor) Claim() error {

	reset := false
	interfaceId := int(m.interfaceId)
	h := m.h

	err := h.ClaimInterface(interfaceId)
	if err != nil {
		log.Warn().
			Err(err).
			Msg("cannot claim interface, resetting device")

		h.ReleaseInterface(interfaceId)
		h.ResetDevice()
		reset = true
		err = nil
	}

	err = h.SetAutoDetachKernelDriver(true)
	if err != nil {
		log.Warn().Msg("failed to set auto detach driver flag")
	}

	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		err = h.ClaimInterface(interfaceId)
		if err == nil {
			log.Debug().Int("interfaceId", interfaceId).Bool("reset", reset).Msg("claim success")
			break
		}
	}

	return err
}

func (m *SmartProUPSMonitor) SendCode(code byte) ([]byte, error) {
	return m.SendCommand([]byte{code})
}

func (m *SmartProUPSMonitor) SendCommand(cmd []byte) ([]byte, error) {

	if m.h == nil {
		return nil, errors.New("handle is not open")
	}

	if len(cmd) > 5 {
		return nil, errors.New("message is too large")
	}

	var csum uint8 = 0
	var done bool = false
	var recv_retries int = 10
	var recv_delay int = 1000
	var reply []byte
	var ret int
	var err error

	buffer := make([]byte, 8)
	buffer[0] = ':'

	var i int = 1
	for _, ch := range cmd {
		buffer[i] = ch
		csum += ch
		i++
	}
	buffer[i] = 255 - csum
	buffer[i+1] = '\r'

	_, err = m.setReport(0, buffer)
	if err != nil {
		return nil, err
	}

	reply = make([]byte, 9)

	err = nil
	for i := 0; i < recv_retries && !done; i++ {
		// TODO: cannot use m.endpointAddress due to type issue
		ret, err = m.h.InterruptTransfer(0x81, reply, 8, recv_delay)
		if ret == len(buffer) && reply[0] == buffer[1] {
			done = true
			err = nil
		} else {
			log.Debug().Int("ret", ret).Int("retry", i).Hex("reply", reply).Send()
		}
	}

	if err != nil && !done {
		log.Warn().Err(err).Msg("read error")
		return nil, err
	}

	if m.debugUSB {
		// Too chatty even for debug
		log.Debug().
			Hex("cmd", buffer).
			Str("cmd_code", string(cmd)).
			Hex("reply", reply).
			Bool("ok", done).
			Send()
	}

	return reply, err
}

func (m *SmartProUPSMonitor) Close() {
	m.closeStream()
	if m.h != nil {
		i := int(m.interfaceId)
		m.h.ReleaseInterface(i)
		ok, err := m.h.KernelDriverActive(i)
		if err == nil && ok {
			m.h.DetachKernelDriver(i)
		}
		m.h.ResetDevice()
		m.h.Close()
	}
	if m.ctx != nil {
		m.ctx.Close()
	}
}

type UPSMetrics struct {
	VendorID              string    `json:"VendorId"`
	ProductID             string    `json:"ProductId"`
	Manufacturer          string    `json:"Manufacturer"`
	Model                 string    `json:"Model"`
	BatteryCharge         float64   `json:"BatteryCharge"`
	BatteryVoltage        float64   `json:"BatteryVoltage"`
	BatteryVoltageNominal float64   `json:"BatteryVoltageNominal"`
	FirmwareVersion       string    `json:"FirmwareVersion"`
	InputFrequency        float64   `json:"InputFrequency"`
	InputFrequencyNominal float64   `json:"InputFrequencyNominal"`
	InputVoltage          float64   `json:"InputVoltage"`
	InputVoltageMaximum   float64   `json:"InputVoltageMaximum"`
	InputVoltageMinimum   float64   `json:"InputVoltageMinimum"`
	InputVoltageNominal   float64   `json:"InputVoltageNominal"`
	Load                  uint      `json:"Load"`
	LoadBanks             int       `json:"LoadBanks"`
	Power                 uint      `json:"PowerNominal"`
	PowerUnit             string    `json:"PowerUnit"`
	Status                string    `json:"Status"`
	TemperatureC          float64   `json:"TempC"`
	TemperatureF          float64   `json:"TempF"`
	UnitId                string    `json:"UnitId"`
	Timestamp             time.Time `json:"Time"`
	UnixTimestamp         int64     `json:"UnixTimestamp"`
}

func (m *SmartProUPSMonitor) closeStream() {
	m.streaming = false
}

func monitorStreamLoop(m *SmartProUPSMonitor, statChan chan *UPSMetrics, errChan chan error, delay time.Duration) {
	log.Info().Dur("delay", delay).Msg("stream started")
	var ms time.Duration = 0 * time.Millisecond
	var delayStep time.Duration = 50 * time.Millisecond

	if delay < time.Duration(delayStep) {
		delay = delayStep
	}

	for m.streaming {
		metrics, err := m.GetStats()
		if err != nil {
			errChan <- err
		} else {
			statChan <- metrics
		}

		for ms = 0; m.streaming && ms < delay; ms += delayStep {
			time.Sleep(delayStep)
		}
	}
}

func (m *SmartProUPSMonitor) ResetInputVoltage() error {
	_, err := m.SendCode('Z')
	return err
}

func (m *SmartProUPSMonitor) OpenStream(delay time.Duration) (chan *UPSMetrics, chan error) {
	metrics := make(chan *UPSMetrics, 1)
	errors := make(chan error, 1)
	go monitorStreamLoop(m, metrics, errors, delay)
	m.streaming = true
	return metrics, errors
}

func (m *SmartProUPSMonitor) tryResetInputVoltageReading() {
	if m.resetVoltageResetEver {
		return
	}

	m.resetVoltageResetEver = true

	err := m.ResetInputVoltage()
	if err != nil {
		log.Error().Err(err).Msg("failed to reset input voltage reading")
	}
}

func (m *SmartProUPSMonitor) GetStats() (*UPSMetrics, error) {

	battery_voltage_nominal := 12.0
	input_voltage_nominal := 120.0
	input_voltage_scaled := 120.0
	switchable_load_banks := 0

	var err error = nil
	now := time.Now()
	metrics := UPSMetrics{Timestamp: now, UnixTimestamp: now.Unix()}
	messages := map[byte][]byte{}
	command_codes := []byte{
		// 'B',
		// 'H',
		// 'X',
		'D', // ok
		'F', // ok
		'L', // ok
		'M', // ok
		'P', // ok
		'S', // ok
		'T', // ok
		'U', // ok
		'V', // ok
	}

	for _, code := range command_codes {
		result, err := m.SendCode(code)
		if err != nil {
			log.Error().Err(err).Str("code", string(code)).Msg("command error")
			continue
		}
		messages[code] = result
	}

	metrics.Manufacturer = m.Manufacturer
	metrics.Model = strings.Replace(m.Product, strings.ToUpper(m.Manufacturer), "", 1)
	metrics.Model = strings.TrimSpace(metrics.Model)
	metrics.VendorID = int_to_hex(m.VendorId)
	metrics.ProductID = int_to_hex(m.ProductId)

	// firmware
	if data, ok := messages['F']; ok {
		tmp := strconv_clean(data[1:7])
		metrics.FirmwareVersion = tmp
	}

	// unit
	if data, ok := messages['U']; ok {
		tmp := (uint64(data[1]) << 8) | uint64(data[2])
		metrics.UnitId = strconv.FormatUint(tmp, 10)
	}

	// load
	if data, ok := messages['L']; ok {
		tmp, _ := strconv.ParseInt(string(data[1:3]), 16, 32)
		metrics.Load = uint(tmp)
	}

	// temp
	if data, ok := messages['T']; ok {
		tmp, _ := strconv.ParseInt(string(data[3:6]), 16, 32)
		freq := float64(tmp) / 10.0
		metrics.InputFrequency = freq

		code := data[6]
		switch code {
		case '0':
			metrics.InputFrequencyNominal = 50
		case '1':
			metrics.InputFrequencyNominal = 60
		}

		tmp, _ = strconv.ParseInt(string(data[1:3]), 16, 32)
		temp := float64(tmp)*0.3636 - 21.0
		tempc := math.Round(temp*100.0) / 100.0
		tempf := math.Round(((temp*(9.0/5.0))+32.0)*100.0) / 100.0
		metrics.TemperatureC = tempc
		metrics.TemperatureF = tempf
	}

	// status
	if data, ok := messages['S']; ok {
		code := data[4]
		if code&4 == 4 {
			metrics.Status = "OFF"
		} else if code&1 == 1 {
			metrics.Status = "OB"
		} else {
			metrics.Status = "OL"
		}
		code = data[1]
		if code == '0' {
			metrics.Status = "LB"
		}
	}

	// voltage
	if data, ok := messages['V']; ok {
		tmp, _ := strconv.ParseInt(string(data[2:4]), 16, 32)
		battery_voltage_nominal = float64(tmp) * 6.0

		ivn := data[1]
		lb := data[4]

		switch ivn {
		case '0':
			input_voltage_nominal = 100.0
			input_voltage_scaled = 100.0
		case '1':
			input_voltage_nominal = 120.0
			input_voltage_scaled = 120.0
		case '2':
			input_voltage_nominal = 230.0
			input_voltage_scaled = 230.0
		case '3':
			input_voltage_nominal = 208.0
			input_voltage_scaled = 230.0
		}

		if lb >= '0' && lb <= '9' {
			switchable_load_banks = int(lb) - '0'
		}

	}
	metrics.LoadBanks = switchable_load_banks
	metrics.InputVoltageNominal = input_voltage_nominal
	metrics.BatteryVoltageNominal = battery_voltage_nominal

	// drain (probably)
	if data, ok := messages['D']; ok {
		tmp, _ := strconv.ParseInt(string(data[1:3]), 16, 32)
		iv := float64(tmp) * input_voltage_scaled / 120.0

		tmp, _ = strconv.ParseInt(string(data[3:5]), 16, 32)
		bv_12v := float64(tmp) / 10.0
		bv := bv_12v * battery_voltage_nominal / 12.0

		metrics.InputVoltage = math.Round(iv*100.0) / 100.0
		metrics.BatteryVoltage = math.Round(bv*100.0) / 100.0

		MIN_VOLT := 11.0
		MAX_VOLT := 13.4
		if bv_12v >= MAX_VOLT {
			metrics.BatteryCharge = 100.0
		} else if bv_12v <= MIN_VOLT {
			metrics.BatteryCharge = 10.0
		} else {
			bc := (100.0 * math.Sqrt((bv_12v-MIN_VOLT)/(MAX_VOLT-MIN_VOLT)))
			metrics.BatteryCharge = math.Round(bc*100.0) / 100.0
		}
		// log.Debug().Float64("bc (12v)", bv_12v).Float64("bc", metrics.BatteryCharge).Send()
	}

	// min / max
	if data, ok := messages['M']; ok {
		tmp, _ := strconv.ParseInt(string(data[1:3]), 16, 32)
		ivmin := float64(tmp) * input_voltage_scaled / 120.0
		metrics.InputVoltageMinimum = math.Round(ivmin*100.0) / 100.0

		// TODO - this value appears always 0, it should be 199
		// log.Info().Float64("ivmin", ivmin).Float64("metrics.InputVoltageMinimum", metrics.InputVoltageMinimum).Float64("tmp", float64(tmp)).Bytes("raw", data[1:3]).Bytes("data", data).Send()
		if ivmin <= 0 {
			m.tryResetInputVoltageReading()
		}

		tmp, _ = strconv.ParseInt(string(data[3:5]), 16, 32)
		ivmax := float64(tmp) * input_voltage_scaled / 120.0
		metrics.InputVoltageMaximum = math.Round(ivmax*100.0) / 100.0
	}

	if data, ok := messages['P']; ok {
		end := bytes.IndexByte(data, 'X')
		va, _ := strconv.ParseUint(string(data[1:end]), 10, 32)
		metrics.Power = uint(va)
		metrics.PowerUnit = "VA"
	}

	return &metrics, err
}

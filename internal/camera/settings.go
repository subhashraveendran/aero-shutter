package camera

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
)

// Choice is one selectable value of a camera setting.
type Choice struct {
	Raw       int64
	Formatted string
}

// Setting is one live camera setting shown in the control panel: the current
// value (raw and human-formatted), whether it can be written, and the values
// the camera accepts.
type Setting struct {
	Code      ptpip.DevicePropCode
	Label     string
	Raw       int64
	Formatted string
	Writable  bool
	Choices   []Choice
}

// controlProps lists the device properties the control panel exposes, in
// display order.
var controlProps = []struct {
	code  ptpip.DevicePropCode
	label string
}{
	{ptpip.PropExposureProgramMode, "Mode"},
	{ptpip.PropFNumber, "Aperture"},
	{ptpip.PropExposureTime, "Shutter"},
	{ptpip.PropExposureIndex, "ISO"},
	{ptpip.PropExposureBias, "Exposure comp."},
	{ptpip.PropWhiteBalance, "White balance"},
	{ptpip.PropStillCaptureMode, "Capture mode"},
	{ptpip.PropBatteryLevel, "Battery"},
}

// propLabel returns the display label for a property code.
func propLabel(code ptpip.DevicePropCode) string {
	for _, p := range controlProps {
		if p.code == code {
			return p.label
		}
	}
	return fmt.Sprintf("0x%04X", uint16(code))
}

// whiteBalanceNames maps PTP WhiteBalance values (standard plus the Nikon
// vendor range) to display names.
var whiteBalanceNames = map[int64]string{
	2:     "Auto",
	4:     "Daylight",
	5:     "Fluorescent",
	6:     "Incandescent",
	7:     "Flash",
	32784: "Cloudy",
	32785: "Shade",
	32786: "Kelvin",
	32787: "Custom",
}

// exposureModeNames maps standard PTP ExposureProgramMode values.
var exposureModeNames = map[int64]string{
	1: "Manual",
	2: "Auto",
	3: "Aperture priority",
	4: "Shutter priority",
}

// captureModeNames maps standard PTP StillCaptureMode values.
var captureModeNames = map[int64]string{
	1: "Single",
	2: "Burst",
	3: "Timelapse",
}

// FormatFNumber renders a PTP FNumber value (f-number x 100) as "f/5.6".
func FormatFNumber(raw int64) string {
	if raw <= 0 {
		return strconv.FormatInt(raw, 10)
	}
	if raw%100 == 0 {
		return fmt.Sprintf("f/%d", raw/100)
	}
	return fmt.Sprintf("f/%.1f", float64(raw)/100)
}

// bulbExposure is the Nikon sentinel ExposureTime for Bulb mode.
const bulbExposure = 0xFFFFFFFF

// FormatExposureTime renders a PTP ExposureTime value (0.1 ms units) as a
// conventional shutter speed such as "1/250s" or "2s".
func FormatExposureTime(raw int64) string {
	if raw == bulbExposure {
		return "Bulb"
	}
	if raw <= 0 {
		return strconv.FormatInt(raw, 10)
	}
	if raw >= 10000 { // one second or longer
		if raw%10000 == 0 {
			return fmt.Sprintf("%ds", raw/10000)
		}
		return fmt.Sprintf("%.1fs", float64(raw)/10000)
	}
	return fmt.Sprintf("1/%ds", int64(math.Round(10000/float64(raw))))
}

// FormatExposureBias renders a PTP ExposureBiasCompensation value
// (millistops) as "+0.7 EV".
func FormatExposureBias(raw int64) string {
	return fmt.Sprintf("%+.1f EV", float64(raw)/1000)
}

// FormatWhiteBalance renders a PTP WhiteBalance value by name, falling back
// to the raw number for unknown (vendor) values.
func FormatWhiteBalance(raw int64) string {
	if name, ok := whiteBalanceNames[raw]; ok {
		return name
	}
	return strconv.FormatInt(raw, 10)
}

// FormatPropValue renders a property value for display according to its
// property code.
func FormatPropValue(code ptpip.DevicePropCode, raw int64) string {
	switch code {
	case ptpip.PropFNumber:
		return FormatFNumber(raw)
	case ptpip.PropExposureTime:
		return FormatExposureTime(raw)
	case ptpip.PropExposureBias:
		return FormatExposureBias(raw)
	case ptpip.PropWhiteBalance:
		return FormatWhiteBalance(raw)
	case ptpip.PropBatteryLevel:
		return fmt.Sprintf("%d%%", raw)
	case ptpip.PropExposureProgramMode:
		if name, ok := exposureModeNames[raw]; ok {
			return name
		}
	case ptpip.PropStillCaptureMode:
		if name, ok := captureModeNames[raw]; ok {
			return name
		}
	}
	return strconv.FormatInt(raw, 10)
}

// maxRangeChoices caps how many discrete choices a range form expands into.
const maxRangeChoices = 40

// rangeChoices expands a {min, max, step} range into at most maxRangeChoices
// evenly stepped raw values, always including min and max.
func rangeChoices(min, max, step int64) []int64 {
	if max < min {
		min, max = max, min
	}
	if step <= 0 {
		step = 1
	}
	count := (max-min)/step + 1
	stride := int64(1)
	if count > maxRangeChoices {
		stride = (count + maxRangeChoices - 1) / maxRangeChoices
	}
	out := make([]int64, 0, maxRangeChoices)
	for v := min; v <= max; v += step * stride {
		out = append(out, v)
	}
	if out[len(out)-1] != max {
		if len(out) >= maxRangeChoices {
			out[len(out)-1] = max
		} else {
			out = append(out, max)
		}
	}
	return out
}

// settingFromDesc builds a Setting from a parsed DevicePropDesc.
func settingFromDesc(desc ptpip.DevicePropDesc) Setting {
	s := Setting{
		Code:      desc.Code,
		Label:     propLabel(desc.Code),
		Raw:       desc.Current.Raw,
		Formatted: FormatPropValue(desc.Code, desc.Current.Raw),
		Writable:  desc.Writable,
	}
	switch desc.FormFlag {
	case ptpip.FormEnum:
		s.Choices = make([]Choice, 0, len(desc.Enum))
		for _, v := range desc.Enum {
			s.Choices = append(s.Choices, Choice{Raw: v.Raw, Formatted: FormatPropValue(desc.Code, v.Raw)})
		}
	case ptpip.FormRange:
		if desc.Range != nil {
			raws := rangeChoices(desc.Range.Min, desc.Range.Max, desc.Range.Step)
			s.Choices = make([]Choice, 0, len(raws))
			for _, r := range raws {
				s.Choices = append(s.Choices, Choice{Raw: r, Formatted: FormatPropValue(desc.Code, r)})
			}
		}
	}
	return s
}

// ReadSettings reads every control-panel property from the camera. Individual
// properties the body does not expose (DevicePropNotSupported and friends)
// are skipped silently; whatever succeeds is returned. The error is non-nil
// only when the context ends.
func (c *Camera) ReadSettings(ctx context.Context) ([]Setting, error) {
	out := make([]Setting, 0, len(controlProps))
	for _, p := range controlProps {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		desc, err := c.client.GetDevicePropDesc(ctx, p.code)
		if err != nil {
			continue
		}
		out = append(out, settingFromDesc(desc))
	}
	return out, nil
}

// ReadSetting reads a single property, used to confirm a value after a write.
func (c *Camera) ReadSetting(ctx context.Context, code ptpip.DevicePropCode) (Setting, error) {
	desc, err := c.client.GetDevicePropDesc(ctx, code)
	if err != nil {
		return Setting{}, fmt.Errorf("camera: read %s: %w", propLabel(code), err)
	}
	return settingFromDesc(desc), nil
}

// SetSetting writes a new raw value for a property. The property is described
// first so the value is encoded in the camera's declared datatype.
func (c *Camera) SetSetting(ctx context.Context, code ptpip.DevicePropCode, raw int64) error {
	desc, err := c.client.GetDevicePropDesc(ctx, code)
	if err != nil {
		return fmt.Errorf("camera: describe %s: %w", propLabel(code), err)
	}
	if !desc.Writable {
		return fmt.Errorf("camera: %s is read-only on this body", propLabel(code))
	}
	if err := c.client.SetDevicePropValue(ctx, code, desc.DataType, ptpip.PropValue{Raw: raw}); err != nil {
		return fmt.Errorf("camera: set %s: %w", propLabel(code), err)
	}
	return nil
}

// TriggerCapture asks the camera to take a photo via InitiateCapture. Bodies
// that do not allow remote capture over Wi-Fi answer with a PTP error, which
// is translated into a readable message.
func (c *Camera) TriggerCapture(ctx context.Context) error {
	if err := c.client.InitiateCapture(ctx); err != nil {
		if ptpip.IsPTPError(err, ptpip.RespOperationNotSupported) {
			return fmt.Errorf("capture not supported over Wi-Fi on this body (%w)", err)
		}
		return fmt.Errorf("camera: trigger capture: %w", err)
	}
	return nil
}

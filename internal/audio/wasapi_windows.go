//go:build windows

package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

const (
	waveFormatIEEEFloat  = 0x0003
	waveFormatExtensible = 0xFFFE
	targetRate           = 16000
)

// List returns active capture endpoints.
func List() ([]Device, error) {
	if err := coInit(); err != nil {
		return nil, err
	}
	defer ole.CoUninitialize()

	enum, err := newEnumerator()
	if err != nil {
		return nil, err
	}
	defer enum.Release()
	return enumDevices(enum)
}

// Open starts capture from the device whose name contains match.
func Open(match string) (Stream, error) {
	devices, err := List()
	if err != nil {
		return nil, err
	}
	dev, err := Pick(devices, match)
	if err != nil {
		return nil, fmt.Errorf("%w\n%s", err, FormatDevices(devices))
	}

	s := &wasapiStream{
		name:   dev.Name,
		chunks: make(chan []float32, 32),
		stop:   make(chan struct{}),
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.run(dev.Name)
		close(s.done)
	}()
	select {
	case err := <-errCh:
		return nil, err
	case <-s.ready:
		return s, nil
	}
}

// FormatDevices renders a device list for logs and errors.
func FormatDevices(devices []Device) string {
	if len(devices) == 0 {
		return "capture devices: (none)"
	}
	out := "capture devices:"
	for _, d := range devices {
		out += "\n  " + d.Name
	}
	return out
}

type wasapiStream struct {
	name   string
	chunks chan []float32
	stop   chan struct{}
	ready  chan struct{}
	done   chan struct{}
	once   sync.Once
}

func (s *wasapiStream) Name() string             { return s.name }
func (s *wasapiStream) SampleRate() int          { return targetRate }
func (s *wasapiStream) Chunks() <-chan []float32 { return s.chunks }

func (s *wasapiStream) Close() error {
	s.once.Do(func() { close(s.stop) })
	<-s.done
	return nil
}

func (s *wasapiStream) run(want string) error {
	defer close(s.chunks)
	if err := coInit(); err != nil {
		return err
	}
	defer ole.CoUninitialize()

	enum, err := newEnumerator()
	if err != nil {
		return err
	}
	defer enum.Release()

	devices, err := enumDevices(enum)
	if err != nil {
		return err
	}
	dev, err := Pick(devices, want)
	if err != nil {
		return err
	}

	var collection *wca.IMMDeviceCollection
	if err := enum.EnumAudioEndpoints(wca.ECapture, wca.DEVICE_STATE_ACTIVE, &collection); err != nil {
		return err
	}
	defer collection.Release()

	mmd, err := deviceByName(collection, dev.Name)
	if err != nil {
		return err
	}
	defer mmd.Release()

	ac, err := activateClient(mmd)
	if err != nil {
		return fmt.Errorf("activate %s: %w", dev.Name, err)
	}
	defer ac.Release()

	var wfx *wca.WAVEFORMATEX
	if err := ac.GetMixFormat(&wfx); err != nil {
		return fmt.Errorf("mix format: %w", err)
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	channels := int(wfx.NChannels)
	rate := int(wfx.NSamplesPerSec)
	bits := int(wfx.WBitsPerSample)
	isFloat := formatIsFloat(wfx)
	if channels < 1 || rate < 1 || bits < 8 {
		return fmt.Errorf("unusable mix format ch=%d rate=%d bits=%d", channels, rate, bits)
	}

	var dur wca.REFERENCE_TIME = 1000000 // 100 ms
	if err := ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, 0, dur, 0, wfx, nil); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	var acc *wca.IAudioCaptureClient
	if err := ac.GetService(wca.IID_IAudioCaptureClient, &acc); err != nil {
		return fmt.Errorf("capture client: %w", err)
	}
	defer acc.Release()

	if err := ac.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	defer ac.Stop()

	resampler := NewResampler(rate, targetRate)
	close(s.ready)

	block := int(wfx.NBlockAlign)
	if block <= 0 {
		block = channels * bits / 8
	}

	for {
		select {
		case <-s.stop:
			return nil
		default:
		}

		var packet uint32
		if err := acc.GetNextPacketSize(&packet); err != nil {
			return err
		}
		if packet == 0 {
			select {
			case <-s.stop:
				return nil
			case <-time.After(20 * time.Millisecond):
			}
			continue
		}

		var data *byte
		var frames, flags uint32
		var devPos, qpc uint64
		if err := acc.GetBuffer(&data, &frames, &flags, &devPos, &qpc); err != nil {
			return err
		}
		var mono []float32
		if flags&wca.AUDCLNT_BUFFERFLAGS_SILENT != 0 || data == nil || frames == 0 {
			mono = make([]float32, int(frames))
		} else {
			nbytes := int(frames) * block
			raw := unsafe.Slice(data, nbytes)
			mono = toMono(raw, channels, bits, isFloat)
		}
		if err := acc.ReleaseBuffer(frames); err != nil {
			return err
		}
		out := resampler.Process(mono)
		if len(out) == 0 {
			continue
		}
		select {
		case <-s.stop:
			return nil
		case s.chunks <- out:
		}
	}
}

func coInit() error {
	err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	if err == nil {
		return nil
	}
	if oe, ok := err.(*ole.OleError); ok && (oe.Code() == 0 || oe.Code() == 1) {
		return nil
	}
	// S_FALSE means the apartment was already initialized.
	if err.Error() == "Incorrect function." {
		return nil
	}
	return err
}

func newEnumerator() (*wca.IMMDeviceEnumerator, error) {
	var enum *wca.IMMDeviceEnumerator
	err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &enum)
	if err != nil {
		return nil, err
	}
	return enum, nil
}

func enumDevices(enum *wca.IMMDeviceEnumerator) ([]Device, error) {
	var collection *wca.IMMDeviceCollection
	if err := enum.EnumAudioEndpoints(wca.ECapture, wca.DEVICE_STATE_ACTIVE, &collection); err != nil {
		return nil, err
	}
	defer collection.Release()

	var count uint32
	if err := collection.GetCount(&count); err != nil {
		return nil, err
	}
	devices := make([]Device, 0, count)
	for i := uint32(0); i < count; i++ {
		var mmd *wca.IMMDevice
		if err := collection.Item(i, &mmd); err != nil {
			return nil, err
		}
		name, err := friendlyName(mmd)
		mmd.Release()
		if err != nil {
			return nil, err
		}
		devices = append(devices, Device{ID: name, Name: name})
	}
	return devices, nil
}

func deviceByName(collection *wca.IMMDeviceCollection, name string) (*wca.IMMDevice, error) {
	var count uint32
	if err := collection.GetCount(&count); err != nil {
		return nil, err
	}
	for i := uint32(0); i < count; i++ {
		var mmd *wca.IMMDevice
		if err := collection.Item(i, &mmd); err != nil {
			return nil, err
		}
		got, err := friendlyName(mmd)
		if err != nil {
			mmd.Release()
			return nil, err
		}
		if got == name {
			return mmd, nil
		}
		mmd.Release()
	}
	return nil, fmt.Errorf("device %q disappeared", name)
}

func friendlyName(mmd *wca.IMMDevice) (string, error) {
	var ps *wca.IPropertyStore
	if err := mmd.OpenPropertyStore(wca.STGM_READ, &ps); err != nil {
		return "", err
	}
	defer ps.Release()
	var pv wca.PROPVARIANT
	if err := ps.GetValue(&wca.PKEY_Device_FriendlyName, &pv); err != nil {
		return "", err
	}
	return pv.String(), nil
}

// activateClient calls Activate with CLSCTX passed by value.
// go-wca v0.3.0 passes a pointer in that slot, which Windows rejects.
func activateClient(mmd *wca.IMMDevice) (*wca.IAudioClient, error) {
	var ac *wca.IAudioClient
	hr, _, _ := syscall.SyscallN(
		mmd.VTable().Activate,
		uintptr(unsafe.Pointer(mmd)),
		uintptr(unsafe.Pointer(wca.IID_IAudioClient)),
		uintptr(wca.CLSCTX_ALL),
		0,
		uintptr(unsafe.Pointer(&ac)),
	)
	if hr != 0 {
		return nil, ole.NewError(hr)
	}
	return ac, nil
}

func formatIsFloat(wfx *wca.WAVEFORMATEX) bool {
	switch wfx.WFormatTag {
	case waveFormatIEEEFloat:
		return true
	case waveFormatExtensible:
		if wfx.CbSize < 22 {
			return wfx.WBitsPerSample == 32
		}
		raw := unsafe.Slice((*byte)(unsafe.Pointer(wfx)), 40)
		sub := binary.LittleEndian.Uint32(raw[24:28])
		return sub == waveFormatIEEEFloat
	default:
		return false
	}
}

func toMono(raw []byte, channels, bits int, isFloat bool) []float32 {
	bps := bits / 8
	if bps <= 0 || channels <= 0 {
		return nil
	}
	frames := len(raw) / (bps * channels)
	out := make([]float32, frames)
	for i := 0; i < frames; i++ {
		var sum float32
		for c := 0; c < channels; c++ {
			off := (i*channels + c) * bps
			sum += decodeSample(raw[off:], bits, isFloat)
		}
		out[i] = sum / float32(channels)
	}
	return out
}

func decodeSample(b []byte, bits int, isFloat bool) float32 {
	if isFloat && bits == 32 && len(b) >= 4 {
		bits := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
		return math.Float32frombits(bits)
	}
	switch bits {
	case 16:
		if len(b) < 2 {
			return 0
		}
		v := int16(uint16(b[0]) | uint16(b[1])<<8)
		return float32(v) / 32768
	case 32:
		if len(b) < 4 {
			return 0
		}
		v := int32(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24)
		return float32(v) / 2147483648
	default:
		return 0
	}
}

//go:build linux

package audio

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
)

const targetRate = 16000

// List returns ALSA capture cards reported by arecord.
func List() ([]Device, error) {
	out, err := exec.Command("arecord", "-l").Output()
	if err != nil {
		return nil, fmt.Errorf("arecord -l: %w", err)
	}
	var devices []Device
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "card ") {
			continue
		}
		var card, dev int
		var name string
		if _, err := fmt.Sscanf(line, "card %d:", &card); err != nil {
			continue
		}
		if i := strings.Index(line, "device "); i >= 0 {
			fmt.Sscanf(line[i:], "device %d:", &dev)
		}
		if i := strings.Index(line, "["); i >= 0 {
			if j := strings.Index(line[i+1:], "]"); j >= 0 {
				name = line[i+1 : i+1+j]
			}
		}
		if name == "" {
			name = line
		}
		id := fmt.Sprintf("plughw:%d,%d", card, dev)
		devices = append(devices, Device{ID: id, Name: name + " (" + id + ")"})
	}
	return devices, nil
}

// FormatDevices renders a device list for logs and errors.
func FormatDevices(devices []Device) string {
	if len(devices) == 0 {
		return "capture devices: (none)"
	}
	out := "capture devices:"
	for _, d := range devices {
		out += "\n  " + d.ID + "  " + d.Name
	}
	return out
}

func toHW(id string) string {
	if strings.HasPrefix(id, "plughw:") {
		return "hw:" + strings.TrimPrefix(id, "plughw:")
	}
	return id
}

// Open starts arecord on the selected device. match may be a name substring
// or an ALSA id such as plughw:1,0.
// Prefers native 48 kHz stereo on hw: (USB codecs), then resamples to 16 kHz
// mono ourselves — plughw SRC is a common fingerprint killer vs phone Shazam.
func Open(match string) (Stream, error) {
	id := strings.TrimSpace(match)
	name := id
	if !strings.Contains(id, ":") {
		devices, err := List()
		if err != nil {
			return nil, err
		}
		dev, err := Pick(devices, match)
		if err != nil {
			return nil, fmt.Errorf("%w\n%s", err, FormatDevices(devices))
		}
		id = dev.ID
		name = dev.Name
	}

	hw := toHW(id)
	if s, err := startArecord(name+" @48k", hw, 48000, true); err == nil {
		log.Printf("capture %s via %s 48k stereo→16k mono", name, hw)
		return s, nil
	} else {
		log.Printf("capture 48k hw failed (%v); falling back to %s 16k", err, id)
	}
	s, err := startArecord(name, id, 16000, false)
	if err != nil {
		return nil, err
	}
	log.Printf("capture %s via %s 16k stereo→mono", name, id)
	return s, nil
}

func startArecord(name, device string, rate int, decimate3 bool) (*arecordStream, error) {
	cmd := exec.Command("arecord", "-D", device, "-f", "S16_LE", "-r", fmt.Sprintf("%d", rate), "-c", "2", "-t", "raw", "-q")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("arecord %s: %w", device, err)
	}
	s := &arecordStream{
		name:      name,
		cmd:       cmd,
		chunks:    make(chan []float32, 32),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		rate:      rate,
		decimate3: decimate3,
	}
	go s.read(stdout)
	return s, nil
}

type arecordStream struct {
	name      string
	cmd       *exec.Cmd
	chunks    chan []float32
	stop      chan struct{}
	done      chan struct{}
	once      sync.Once
	chLogged  sync.Once
	rate      int
	decimate3 bool
}

func (s *arecordStream) Name() string             { return s.name }
func (s *arecordStream) SampleRate() int          { return targetRate }
func (s *arecordStream) Chunks() <-chan []float32 { return s.chunks }

func (s *arecordStream) Close() error {
	s.once.Do(func() { close(s.stop) })
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	<-s.done
	return nil
}

func (s *arecordStream) read(r io.Reader) {
	defer close(s.chunks)
	defer close(s.done)
	defer s.cmd.Wait()
	br := bufio.NewReaderSize(r, 16384)
	// 100 ms of stereo s16le at capture rate.
	frames := s.rate / 10
	if frames < 160 {
		frames = 1600
	}
	raw := make([]byte, frames*2*2)
	for {
		select {
		case <-s.stop:
			return
		default:
		}
		if _, err := io.ReadFull(br, raw); err != nil {
			return
		}
		stereo := make([]float32, len(raw)/2)
		for i := range stereo {
			v := int16(binary.LittleEndian.Uint16(raw[i*2:]))
			stereo[i] = float32(v) / 32768
		}
		choice := ChannelChoice(stereo)
		s.chLogged.Do(func() {
			log.Printf("capture channel: %s (stereo→mono)", choice)
		})
		mono := StereoToMono(stereo)
		if s.decimate3 {
			mono = Decimate3(mono)
		}
		select {
		case <-s.stop:
			return
		case s.chunks <- mono:
		}
	}
}

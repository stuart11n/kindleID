package solaredge

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"time"
)

// ModbusOptions configures a local SolarEdge Modbus TCP inverter.
type ModbusOptions struct {
	Addr   string // host:port, e.g. 192.168.74.61:502
	UnitID byte   // usually 1
}

// ModbusClient reads production and battery over Modbus TCP.
type ModbusClient struct {
	opts ModbusOptions
}

func NewModbus(opts ModbusOptions) *ModbusClient {
	if opts.UnitID == 0 {
		opts.UnitID = 1
	}
	return &ModbusClient{opts: opts}
}

func (c *ModbusClient) Enabled() bool {
	addr := c.opts.Addr
	return addr != "" && addr != "SOLAREDGE_HOST:502"
}

// Fetch connects, reads SunSpec + StorEdge registers, then disconnects.
// SolarEdge allows only one Modbus TCP client at a time.
func (c *ModbusClient) Fetch(ctx context.Context) (Snapshot, error) {
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", c.opts.Addr)
	if err != nil {
		return Snapshot{}, fmt.Errorf("modbus dial %s: %w", c.opts.Addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	regs, err := readHolding(conn, c.opts.UnitID, 40000, 70)
	if err != nil {
		return Snapshot{}, fmt.Errorf("sunspec common: %w", err)
	}
	if len(regs) < 2 || regs[0] != 0x5375 || regs[1] != 0x6e53 {
		return Snapshot{}, fmt.Errorf("modbus: not a SunSpec map at 40000 (got %04x %04x)", regs[0], regs[1])
	}

	inv, err := readHolding(conn, c.opts.UnitID, 40069, 40)
	if err != nil {
		return Snapshot{}, fmt.Errorf("inverter block: %w", err)
	}
	// Inverter model starts at 40069 (0-based). Within that block:
	// 40083 I_AC_Power is offset 14 from 40069; 40084 SF offset 15
	// 40093-40094 lifetime Wh offset 24-25; 40095 SF offset 26
	acPower := scaleInt16(int16(inv[14]), int16(inv[15]))
	lifetime := scaleUint32(uint32(inv[24])<<16|uint32(inv[25]), int16(inv[26]))

	var battWatts float64 // +charge / −discharge
	snap := Snapshot{
		SiteName:    "Solar",
		PVWatts:     acPower,
		LifetimeWh: lifetime,
		TodayWh:     0, // not available as a simple daily counter over Modbus
		Updated:     time.Now(),
	}

	// StorEdge battery floats (try primary map, then alternate).
	if soc, watts, ok := readBattery(conn, c.opts.UnitID, 57716, 57732); ok {
		snap.HasBattery = true
		snap.BatteryPct = int(math.Round(soc))
		snap.BatteryWatts = math.Abs(watts)
		snap.BatteryCharging = watts > 1
		snap.BatteryCritical = snap.BatteryPct > 0 && snap.BatteryPct < 10
		battWatts = watts
	} else if soc, watts, ok := readBattery(conn, c.opts.UnitID, 17709, 17707); ok {
		snap.HasBattery = true
		snap.BatteryPct = int(math.Round(soc))
		snap.BatteryWatts = math.Abs(watts)
		snap.BatteryCharging = watts > 1
		snap.BatteryCritical = snap.BatteryPct > 0 && snap.BatteryPct < 10
		battWatts = watts
	}

	// I_AC_Power is inverter AC output, which includes battery discharge at night.
	// True PV ≈ AC + battery(+charge/−discharge).
	if snap.HasBattery {
		pv := acPower + battWatts
		if pv < 0 {
			pv = 0
		}
		snap.PVWatts = pv
	}

	// Walk SunSpec for meter model 201–204; M_AC_Power at +18, SF at +22.
	// Positive meter power = export to grid (SunSpec / SolarEdge).
	if grid, ok := readMeterPower(conn, c.opts.UnitID); ok {
		snap.HasMeter = true
		snap.GridWatts = math.Abs(grid)
		snap.GridExport = grid > 0
		// Home load is AC-side balance: inverter AC − signed grid.
		// Do not subtract battery again — I_AC_Power already excludes charge
		// (and includes discharge), so PV − grid − batt ≡ acPower − grid.
		load := acPower - grid
		if load < 0 {
			load = 0
		}
		snap.LoadWatts = load
	}

	return snap, nil
}

func readMeterPower(conn net.Conn, unit byte) (watts float64, ok bool) {
	addr := uint16(40002)
	for i := 0; i < 12; i++ {
		hdr, err := readHolding(conn, unit, addr, 2)
		if err != nil || len(hdr) < 2 {
			return 0, false
		}
		did, length := hdr[0], hdr[1]
		if did == 0xFFFF || length == 0 {
			return 0, false
		}
		if did >= 201 && did <= 204 && length >= 23 {
			block, err := readHolding(conn, unit, addr, 24)
			if err != nil || len(block) < 23 {
				return 0, false
			}
			return scaleInt16(int16(block[18]), int16(block[22])), true
		}
		addr += 2 + length
	}
	return 0, false
}

func readBattery(conn net.Conn, unit byte, powerAddr, socAddr uint16) (soc, watts float64, ok bool) {
	pwrRegs, err := readHolding(conn, unit, powerAddr, 2)
	if err != nil {
		return 0, 0, false
	}
	socRegs, err := readHolding(conn, unit, socAddr, 2)
	if err != nil {
		return 0, 0, false
	}
	watts = float32FromRegs(pwrRegs[0], pwrRegs[1])
	soc = float32FromRegs(socRegs[0], socRegs[1])
	if math.IsNaN(soc) || soc < 0 || soc > 100 {
		return 0, 0, false
	}
	if math.IsNaN(watts) {
		watts = 0
	}
	return soc, watts, true
}

func scaleInt16(v, sf int16) float64 {
	return float64(v) * math.Pow10(int(sf))
}

func scaleUint32(v uint32, sf int16) float64 {
	return float64(v) * math.Pow10(int(sf))
}

func float32FromRegs(first, second uint16) float64 {
	// SolarEdge StorEdge floats are word-swapped IEEE754 (low word first).
	bits := uint32(second)<<16 | uint32(first)
	return float64(math.Float32frombits(bits))
}

func readHolding(conn net.Conn, unit byte, addr, quantity uint16) ([]uint16, error) {
	var txn uint16 = 1
	// MBAP + PDU: txn(2) proto(2) len(2) unit(1) fc(1) addr(2) qty(2)
	req := make([]byte, 12)
	binary.BigEndian.PutUint16(req[0:], txn)
	binary.BigEndian.PutUint16(req[2:], 0) // protocol
	binary.BigEndian.PutUint16(req[4:], 6) // length
	req[6] = unit
	req[7] = 0x03 // read holding
	binary.BigEndian.PutUint16(req[8:], addr)
	binary.BigEndian.PutUint16(req[10:], quantity)
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	hdr := make([]byte, 7)
	if _, err := readFull(conn, hdr); err != nil {
		return nil, err
	}
	if binary.BigEndian.Uint16(hdr[2:]) != 0 {
		return nil, fmt.Errorf("bad protocol id")
	}
	length := binary.BigEndian.Uint16(hdr[4:])
	if length < 2 {
		return nil, fmt.Errorf("short frame")
	}
	pdu := make([]byte, int(length)-1) // unit already in hdr[6]
	if _, err := readFull(conn, pdu); err != nil {
		return nil, err
	}
	if pdu[0] == 0x83 {
		return nil, fmt.Errorf("modbus exception %d", pdu[1])
	}
	if pdu[0] != 0x03 {
		return nil, fmt.Errorf("unexpected fc %d", pdu[0])
	}
	byteCount := int(pdu[1])
	if byteCount+2 > len(pdu) || byteCount%2 != 0 {
		return nil, fmt.Errorf("bad byte count %d", byteCount)
	}
	out := make([]uint16, byteCount/2)
	for i := 0; i < len(out); i++ {
		out[i] = binary.BigEndian.Uint16(pdu[2+i*2:])
	}
	return out, nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	n := 0
	for n < len(buf) {
		c, err := conn.Read(buf[n:])
		n += c
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

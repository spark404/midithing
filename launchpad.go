package main

import (
	"bytes"
	"fmt"
	"github.com/spark404/go-coremidi"
	"log"
	"math"
	"time"
)

type Launchpad struct {
	client *coremidi.Client

	inPort  coremidi.InputPort
	outPort coremidi.OutputPort

	portConnection coremidi.PortConnection

	destination coremidi.Destination

	statusChannel         <-chan GrblStatus
	positionChangeChannel chan<- GrblPositionChangeRequest

	currentState string
}

type StandaloneLayout byte

const (
	Note       StandaloneLayout = 0
	Drum       StandaloneLayout = 1
	Fader      StandaloneLayout = 2
	Programmer StandaloneLayout = 3
)

func (s StandaloneLayout) AsByte() byte {
	return byte(s)
}

type SysExParameter byte

const (
	SetLEDs                SysExParameter = 0x0A
	SetLEDsRGB             SysExParameter = 0x0B
	SetLEDsByColumn        SysExParameter = 0x0C
	SetLEDsByRow           SysExParameter = 0x0D
	SetAllLEDs             SysExParameter = 0x0E
	SetAllLEDsInGridRGB    SysExParameter = 0x0F
	ScrollText             SysExParameter = 0x14
	ModeSelection          SysExParameter = 0x21
	ModeStatus             SysExParameter = 0x2D
	FlashLED               SysExParameter = 0x23
	PulseLED               SysExParameter = 0x28
	FaderSetup             SysExParameter = 0x2B
	StandaloneLayoutSelect SysExParameter = 0x2C
	StandaloneLayoutStatus SysExParameter = 0x2F
)

func (s SysExParameter) AsByte() byte {
	return byte(s)
}

type Button byte

const (
	Record     Button = 0x0A
	ArrowUp    Button = 0x5B
	ArrowDown  Button = 0x5C
	ArrowLeft  Button = 0x5D
	ArrowRight Button = 0x5E
)

func (b Button) asByte() byte {
	return byte(b)
}

func NewLaunchpad(statusChannel <-chan GrblStatus, positionChange chan<- GrblPositionChangeRequest) (*Launchpad, error) {
	launchpad := Launchpad{}
	if err := launchpad.init(); err != nil {
		return nil, fmt.Errorf("failed to init launchpad: %+w", err)
	}

	launchpad.statusChannel = statusChannel
	launchpad.positionChangeChannel = positionChange

	go launchpad.statusUpdate()

	return &launchpad, nil
}

func (l *Launchpad) init() error {
	client, err := coremidi.NewClient("midithing")
	if err != nil {
		return fmt.Errorf("coremidi error: %w", err)
	}

	devices, err := coremidi.AllDevices()
	if err != nil {
		return fmt.Errorf("coremidi error: %w", err)
	}

	launchpads := Filter(devices, func(v coremidi.Device) bool {
		return "Launchpad Pro" == v.Name()
	})

	if len(launchpads) == 0 {
		return fmt.Errorf("device not found")
	}

	launchpad := launchpads[0] // Use the first one regardless
	log.Printf("Found %v : %v\n", launchpad.Manufacturer(), launchpad.Name())

	if launchpad.IsOffline() {
		return fmt.Errorf("device offline")
	}

	entities, err := launchpad.Entities()
	if err != nil {
		return fmt.Errorf("coremidi error: %w", err)
	}

	entities = FindEntityByName(entities, "Standalone Port")
	if len(entities) == 0 {
		return fmt.Errorf("standalone port not found")
	}

	entity := entities[0]

	destinations, err := entity.Destinations()
	if err != nil {
		return fmt.Errorf("coremidi error: %w", err)
	}

	l.destination = destinations[0]

	log.Printf("Connecting to %s using destination %s\n", launchpad.Name(), l.destination.Name())

	l.outPort, err = coremidi.NewOutputPort(client, "Midithing Out")
	if err != nil {
		return fmt.Errorf("coremidi error: %w", err)
	}

	l.inPort, err = coremidi.NewInputPort(client, "Midithing In", l.onMessage)
	if err != nil {
		return fmt.Errorf("coremidi error: %w", err)
	}

	sources, err := entity.Sources()
	if err != nil {
		return fmt.Errorf("coremidi error: %w", err)
	}

	if len(sources) <= 0 {
		return fmt.Errorf("no sources availale on entity %s: %w", entity.Name(), err)
	}

	if l.portConnection, err = l.inPort.Connect(sources[0]); err != nil {
		return fmt.Errorf("coremidi error: %w", err)
	}

	if err := l.SetStandaloneLayout(Programmer); err != nil {
		return fmt.Errorf("coremidi error: %w", err)
	}

	buffer := createSysExMessage([]byte{
		SetAllLEDs.AsByte(),
		0x00,
	})
	setAllPacket := coremidi.NewPacket(buffer, now())
	err = setAllPacket.Send(&l.outPort, &l.destination)

	buffer = createSysExMessage([]byte{
		SetLEDs.AsByte(),
		ArrowUp.asByte(), 0x4B,
		ArrowDown.asByte(), 0x4B,
		ArrowLeft.asByte(), 0x4B,
		ArrowRight.asByte(), 0x4B,
	})
	setArrowsPacket := coremidi.NewPacket(buffer, now())
	err = setArrowsPacket.Send(&l.outPort, &l.destination)
	if err != nil {
		return fmt.Errorf("coremidi error: %w", err)
	}

	return nil
}

func (l *Launchpad) statusUpdate() {
	for {
		select {
		case status := <-l.statusChannel:
			if l.currentState != status.state {
				if err := l.setState(status.state); err != nil {
					log.Printf("failed to set state: %v", err)
				}
				l.currentState = status.state
			}

			if err := l.setPosition(status.position.X, status.position.Y); err != nil {
				log.Printf("failed to set X position: %v", err)
			}
		}
	}
}

func (l *Launchpad) setState(status string) error {
	color := 0
	pulseColor := 0

	switch status {
	case "Idle":
		color = 96
		pulseColor = 96
	case "Run":
		color = 75
		pulseColor = 76
	case "Alarm":
		color = 5
		pulseColor = 0
	default:
		return fmt.Errorf("no state %s", status)
	}

	buffer := createSysExMessage([]byte{
		SetLEDs.AsByte(),
		Record.asByte(),
		byte(color),
	})

	packet := coremidi.NewPacket(buffer, now())
	_ = packet.Send(&l.outPort, &l.destination)

	if color != pulseColor {
		buffer := createSysExMessage([]byte{
			PulseLED.AsByte(),
			Record.asByte(),
			byte(color),
		})

		packet = coremidi.NewPacket(buffer, 1235)
		_ = packet.Send(&l.outPort, &l.destination)
	}

	return nil
}

func (l *Launchpad) setPosition(x float64, y float64) error {
	currentX := int(math.Round(x / 100))
	currentY := int(math.Round(y / 100))

	var matrix [8][8]byte // rows / cols
	for row := range matrix {
		for column := range matrix[row] {
			matrix[row][column] = 127
			if currentY == row {
				matrix[row][column] = 13
				if currentX == column {
					matrix[row][column] = 5
				}
			}
		}
	}
	_ = l.setRowColor(matrix)

	return nil
}

func (l *Launchpad) setRowColor(matrix [8][8]byte) error {
	var setColor bytes.Buffer
	setColor.WriteByte(SetLEDs.AsByte())

	for row := range matrix {
		firstIndex := (row+1)*10 + 1
		for column := range matrix[row] {
			setColor.WriteByte(byte(firstIndex + column))
			setColor.WriteByte(matrix[row][column])
		}
	}

	setAllPacket := coremidi.NewPacket(createSysExMessage(setColor.Bytes()), now())
	return setAllPacket.Send(&l.outPort, &l.destination)
}

func (l *Launchpad) onMessage(source coremidi.Source, packet coremidi.Packet) {
	if packet.Data[0] == 0xb0 { // CC

		if packet.Data[2] != 127 {
			// Ignore everything but "button down"
			return
		}

		switch Button(packet.Data[1]) {
		case ArrowUp:
			l.positionChangeChannel <- GrblPositionChangeRequest{
				Y:        1,
				Relative: true,
			}
		case ArrowDown:
			l.positionChangeChannel <- GrblPositionChangeRequest{
				Y:        -1,
				Relative: true,
			}
		case ArrowLeft:
			l.positionChangeChannel <- GrblPositionChangeRequest{
				X:        -1,
				Relative: true,
			}
		case ArrowRight:
			l.positionChangeChannel <- GrblPositionChangeRequest{
				X:        1,
				Relative: true,
			}
		}
	} else if packet.Data[0] == 0x90 && packet.Data[2] > 0 { // Note On
		row := packet.Data[1] / 10
		column := packet.Data[1] % 10

		if row < 1 || row > 8 || column < 1 || column > 8 {
			log.Printf("Row/Column index out of bounds")
		}

		log.Printf("Pressed row %d / col %d", row, column)
		positionChange := GrblPositionChangeRequest{
			X:        int(column - 1),
			Y:        int(row - 1),
			Z:        0,
			Relative: false,
		}

		log.Printf("Sending position change: %v", positionChange)
		l.positionChangeChannel <- positionChange
	}
}

func (l *Launchpad) Close() {
	log.Printf("Resetting to Note layout")
	if err := l.SetStandaloneLayout(Note); err != nil {
		log.Printf("Error: %v", err)
	}

	log.Printf("Disconnecting from MIDI sources")
	l.portConnection.Disconnect()
}

func (l *Launchpad) SetStandaloneLayout(standaloneLayout StandaloneLayout) error {
	buffer := createSysExMessage([]byte{
		StandaloneLayoutSelect.AsByte(),
		standaloneLayout.AsByte(),
	})

	modeChangePacket := coremidi.NewPacket(buffer, now())
	if err := modeChangePacket.Send(&l.outPort, &l.destination); err != nil {
		return fmt.Errorf("error while sending packet: %w", err)
	}

	return nil
}

func createSysExMessage(data []byte) []byte {
	var sysEx bytes.Buffer
	sysEx.Write([]byte{0xF0, 0x00, 0x20, 0x29, 0x02, 0x10})
	sysEx.Write(data)
	sysEx.Write([]byte{0xF7})

	return sysEx.Bytes()
}

func now() uint64 {
	return uint64(time.Now().Unix())
}

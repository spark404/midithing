package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/spark404/go-coremidi"
	"log"
	"math"
)

type Launchpad struct {
	client      *coremidi.Client
	outPort     coremidi.OutputPort
	destination coremidi.Destination

	statusChannel         <-chan GrblStatus
	positionChangeChannel chan<- GrblPositionChangeRequest

	currentState string
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
		return fmt.Errorf("failed to init coremidi client: %+w", err)
	}

	devices, err := coremidi.AllDevices()
	if err != nil {
		log.Fatal(err)
	}

	launchpads := Filter(devices, func(v coremidi.Device) bool {
		return "Launchpad Pro" == v.Name()
	})

	if len(launchpads) == 0 {
		log.Fatal("Device \"Launchpad Pro\" not found")
	}

	launchpad := launchpads[0] // Use the first one regardless
	log.Printf("Found %v : %v\n", launchpad.Manufacturer(), launchpad.Name())

	if launchpad.IsOffline() {
		log.Fatalf("Device is not online\n")
	}

	entities, err := launchpad.Entities()
	if err != nil {
		log.Fatal(err)
	}

	entities = FindEntityByName(entities, "Standalone Port")
	if len(entities) == 0 {
		log.Fatal("Failed to find port")
	}

	entity := entities[0]

	destinations, err := entity.Destinations()
	if err != nil {
		log.Fatal(err)
	}

	l.destination = destinations[0]

	log.Printf("Connecting to %s using destination %s\n", launchpad.Name(), l.destination.Name())

	l.outPort, err = coremidi.NewOutputPort(client, "Midithing Out")
	if err != nil {
		log.Fatal(err)
	}

	inPort, err := coremidi.NewInputPort(client, "Midithing In", l.onMessage)
	if err != nil {
		log.Fatal(err)
	}

	sources, err := entity.Sources()
	if err != nil {
		log.Fatalf("Failed to query sources on entity %s: %v", entity.Name(), err)
	}

	if len(sources) <= 0 {
		log.Fatalf("No sources availale on entity %s: %v", entity.Name(), err)
	}

	if _, err := inPort.Connect(sources[0]); err != nil {
		log.Fatalf("Failed to connect input to source %s: %v", sources[0].Name(), err)
	}

	var modeChange bytes.Buffer
	modeChange.WriteByte(0xF0)
	modeChange.WriteByte(0x00)
	modeChange.WriteByte(0x20)
	modeChange.WriteByte(0x29)
	modeChange.WriteByte(0x02)
	modeChange.WriteByte(0x10)
	modeChange.WriteByte(0x2C)
	modeChange.WriteByte(0x03) //Programmer layout 0x03
	modeChange.WriteByte(0xF7)

	modeChangePacket := coremidi.NewPacket(modeChange.Bytes(), 1234)
	err = modeChangePacket.Send(&l.outPort, &l.destination)

	if err != nil {
		log.Fatal(err)
	}

	var setAll bytes.Buffer
	setAll.WriteByte(0xF0)
	setAll.WriteByte(0x00)
	setAll.WriteByte(0x20)
	setAll.WriteByte(0x29)
	setAll.WriteByte(0x02)
	setAll.WriteByte(0x10)
	setAll.WriteByte(0x0E) // set all
	setAll.WriteByte(0x00) // color
	setAll.WriteByte(0xF7)

	setAllPacket := coremidi.NewPacket(setAll.Bytes(), 1234)
	err = setAllPacket.Send(&l.outPort, &l.destination)

	var setArrows bytes.Buffer
	setArrows.WriteByte(0xF0)
	setArrows.WriteByte(0x00)
	setArrows.WriteByte(0x20)
	setArrows.WriteByte(0x29)
	setArrows.WriteByte(0x02)
	setArrows.WriteByte(0x10)
	setArrows.WriteByte(0x0A)
	setArrows.WriteByte(0x5B)
	setArrows.WriteByte(0x4B)
	setArrows.WriteByte(0x5C)
	setArrows.WriteByte(0x4B)
	setArrows.WriteByte(0x5D)
	setArrows.WriteByte(0x4B)
	setArrows.WriteByte(0x5E)
	setArrows.WriteByte(0x4B)
	setArrows.WriteByte(0xF7)

	setArrowsPacket := coremidi.NewPacket(setArrows.Bytes(), 1234)
	err = setArrowsPacket.Send(&l.outPort, &l.destination)

	if err != nil {
		log.Fatal(err)
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
	default:
		return fmt.Errorf("no state %s", status)
	}
	var setColor bytes.Buffer
	setColor.WriteByte(0xF0)
	setColor.WriteByte(0x00)
	setColor.WriteByte(0x20)
	setColor.WriteByte(0x29)
	setColor.WriteByte(0x02)
	setColor.WriteByte(0x10)
	setColor.WriteByte(0x0A)
	setColor.WriteByte(0x0A)
	setColor.WriteByte(byte(color)) // color
	setColor.WriteByte(0xF7)

	packet := coremidi.NewPacket(setColor.Bytes(), 1234)
	_ = packet.Send(&l.outPort, &l.destination)

	if color != pulseColor {
		var setPulseColor bytes.Buffer
		setPulseColor.WriteByte(0xF0)
		setPulseColor.WriteByte(0x00)
		setPulseColor.WriteByte(0x20)
		setPulseColor.WriteByte(0x29)
		setPulseColor.WriteByte(0x02)
		setPulseColor.WriteByte(0x10)
		setPulseColor.WriteByte(0x28)
		setPulseColor.WriteByte(0x0A)
		setPulseColor.WriteByte(byte(pulseColor)) // color
		setPulseColor.WriteByte(0xF7)

		packet = coremidi.NewPacket(setPulseColor.Bytes(), 1235)
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
	setColor.WriteByte(0xF0)
	setColor.WriteByte(0x00)
	setColor.WriteByte(0x20)
	setColor.WriteByte(0x29)
	setColor.WriteByte(0x02)
	setColor.WriteByte(0x10)
	setColor.WriteByte(0x0A)

	for row := range matrix {
		firstIndex := (row+1)*10 + 1
		for column := range matrix[row] {
			setColor.WriteByte(byte(firstIndex + column))
			setColor.WriteByte(matrix[row][column])
		}
	}

	setColor.WriteByte(0xF7)

	setAllPacket := coremidi.NewPacket(setColor.Bytes(), 1234)
	return setAllPacket.Send(&l.outPort, &l.destination)
}

func (l *Launchpad) onMessage(source coremidi.Source, packet coremidi.Packet) {
	if packet.Data[0] == 0xb0 { // CC
		if packet.Data[2] == 127 {
			return
		}

		// Arrow buttons, relative movement
		positionChange := GrblPositionChangeRequest{
			Relative: true,
		}

		switch packet.Data[1] {
		case 0x5b: // Up
			positionChange.Y = 1
		case 0x5c: // Down
			positionChange.Y = -1
		case 0x5d: // Left
			positionChange.X = -1
		case 0x5e: // Right
			positionChange.X = 1
		}

		log.Printf("Sending position change: %v", positionChange)
		l.positionChangeChannel <- positionChange
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
	} else {
		log.Printf("Got something from source %s", source.Name())
		log.Print(hex.Dump(packet.Data))
	}

}

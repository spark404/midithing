package main

import (
	"bytes"
	"fmt"
	"github.com/glycerine/rbuf"
	"go.bug.st/serial"
	"log"
	"strings"
	"time"
)

type SerialConnection struct {
	serialPort  serial.Port
	readChannel chan<- string
	ringBuffer  *rbuf.AtomicFixedSizeRingBuf
}

func NewSerialConnection(reader chan<- string) (*SerialConnection, error) {
	serialConnection := SerialConnection{}
	serialConnection.readChannel = reader

	if err := serialConnection.init(); err != nil {
		return nil, fmt.Errorf("failed to init serial port %w", err)
	}

	serialConnection.ringBuffer = rbuf.NewAtomicFixedSizeRingBuf(1024)

	// grbl init stuff
	serialConnection.serialPort.Write([]byte{'\r', '\n', '\r', '\n'})
	time.Sleep(2 * time.Second)
	temp := make([]byte, 1024)
	serialConnection.serialPort.Read(temp) // poor mans flush

	go serialConnection.reader()

	return &serialConnection, nil
}

func (s *SerialConnection) init() error {
	mode := &serial.Mode{
		BaudRate: 115200,
	}

	var err error
	s.serialPort, err = serial.Open("/dev/cu.usbmodem14101", mode)
	if err != nil {
		return fmt.Errorf("failed to open serial port: %w", err)
	}

	return nil
}

func (s *SerialConnection) reader() {
	shutdown := false

	for !shutdown {
		buffer := make([]byte, 80)
		n, err := s.serialPort.Read(buffer)
		if err != nil {
			log.Printf("Error while reading from serial port: %v", err)
			shutdown = true
		}

		// Add data to the ring buffer
		s.ringBuffer.Write(buffer[:n])

		peek := make([]byte, 1024)
		s.ringBuffer.ReadWithoutAdvance(peek)
		i := bytes.Index(peek, []byte{'\r', '\n'})

		if i == -1 {
			continue
		}

		s.ringBuffer.Advance(i + 2)
		log.Printf("serial IN: '%s' (%d bytes)", peek[:i], len(peek[:i]))
		s.readChannel <- string(peek[:i])
	}
}

func (s *SerialConnection) Write(data string) (int, error) {
	buffer := []byte(data)

	size := len(buffer)
	bytesWritten := 0
	for bytesWritten < size {
		n, err := s.serialPort.Write(buffer)
		if err != nil {
			return bytesWritten, fmt.Errorf("failed to write to serial port: %+w", err)
		}

		bytesWritten = bytesWritten + n
	}

	log.Printf("serial OUT: '%s' (%d bytes)", strings.TrimRight(data, "\r\n"), bytesWritten)
	return bytesWritten, nil
}

func (s *SerialConnection) Close() error {
	log.Println("Closing serial connection")

	if err := s.serialPort.Close(); err != nil {
		log.Printf("error during close: %v", err)
	}

	return nil
}

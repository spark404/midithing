package main

import (
	"fmt"
	"github.com/spark404/go-coremidi"
	"log"
	strconv2 "strconv"
	"strings"
	"time"
)

type Playback struct {
	Group   string
	Page    int
	Index   int
	TitanId int
	Active  bool
}

type GrblStatus struct {
	state    string
	position struct {
		X float64
		Y float64
		Z float64
	}
}

type GrblPositionChangeRequest struct {
	X        int
	Y        int
	Z        int
	Relative bool
}

type GrblCommandStack struct {
	rxbufferRemaining int
	commands          []string
}

func (g *GrblCommandStack) Push(command string) (int, error) {
	g.commands = append(g.commands, command)
	g.rxbufferRemaining -= len(command)

	log.Printf("Push command '%s' on stack, %d remaining", command, g.rxbufferRemaining)
	return len(command), nil
}

func (g *GrblCommandStack) Pop() error {
	if len(g.commands) == 0 {
		return fmt.Errorf("no commands to pop")
	}

	poppedCommand := g.commands[0]
	g.commands = g.commands[1:]

	g.rxbufferRemaining += len(poppedCommand)

	log.Printf("Pop command '%s' from stack, %d remaining", poppedCommand, g.rxbufferRemaining)
	return nil
}

func (g *GrblCommandStack) CanPush(command string) bool {
	return g.rxbufferRemaining >= len(command)
}

func main() {
	serialData := make(chan string)
	statusUpdate := make(chan GrblStatus, 2)
	positionChange := make(chan GrblPositionChangeRequest, 2)

	serialConnection, err := NewSerialConnection(serialData)
	if err != nil {
		log.Fatal(err)
	}

	_, err = NewLaunchpad(statusUpdate, positionChange)
	if err != nil {
		log.Fatal(err)
	}

	stack := GrblCommandStack{
		rxbufferRemaining: 126,
	}

	tick := time.Tick(1 * time.Second)

	for {
		select {
		case message := <-serialData:
			log.Printf("IN: %s", message)
			if message[0] == '<' {
				status, err := ParseStatus(message)
				if err != nil {
					log.Printf("failed to parse status: %v", err)
					continue
				}

				statusUpdate <- *status
			} else if message[0] == 'o' && message[1] == 'k' {
				stack.Pop()
			}
		case delta := <-positionChange:
			var command string
			if delta.Relative {
				command = fmt.Sprintf("G91 X%.3f Y%.3f\r", float64(delta.X*100), float64(delta.Y*100))
			} else {
				command = fmt.Sprintf("G90 X%.3f Y%.3f\r", float64(delta.X*100), float64(delta.Y*100))
			}
			if stack.CanPush(command) {
				if _, err := serialConnection.Write(command); err != nil {
					log.Printf("Failed to send command to serial connection: %v", err)
					continue
				}
				stack.Push(command)
			}
		case <-tick:
			n, err := serialConnection.Write("?")
			if err != nil {
				log.Printf("write error: %v", err)
			}

			if n != 1 {
				log.Println("huh")
			}
		default:
			// Pacing, do we need this?
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func Filter(vs []coremidi.Device, f func(coremidi.Device) bool) []coremidi.Device {
	vsf := make([]coremidi.Device, 0)
	for _, v := range vs {
		if f(v) {
			vsf = append(vsf, v)
		}
	}
	return vsf
}

func FindEntityByName(entities []coremidi.Entity, needle string) []coremidi.Entity {
	vsf := make([]coremidi.Entity, 0)
	for _, element := range entities {
		if element.Name() == needle {
			vsf = append(vsf, element)
		}
	}

	return vsf
}

func ParseStatus(rawMessage string) (*GrblStatus, error) {
	components := strings.Split(removeMarkers(rawMessage), "|")
	if len(components) < 2 {
		return nil, fmt.Errorf("failed to properly split the status string")
	}

	grblStatus := GrblStatus{}
	for i, component := range components {
		if i == 0 {
			grblStatus.state = component
		} else {
			kv := strings.Split(component, ":")
			if kv[0] == "MPos" {
				values := strings.Split(kv[1], ",")
				grblStatus.position.X, _ = strconv2.ParseFloat(values[0], 32)
				grblStatus.position.Y, _ = strconv2.ParseFloat(values[1], 32)
				grblStatus.position.Z, _ = strconv2.ParseFloat(values[2], 32)
			} else {
				// log.Printf("not handling %s yet", kv[0])
			}
		}
	}

	return &grblStatus, nil
}

func removeMarkers(s string) string {
	r := []rune(s)
	return string(r[1 : len(r)-1])
}
